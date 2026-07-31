package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/rs/zerolog"

	"github.com/YugaAI/PR-Summarizer-CLI/internal/domain"
)

const (
	mimoModel   = "mimo-v2.5-pro"
	mimoBaseURL = "https://api.xiaomimimo.com/anthropic"
)

type MimoClient struct {
	client  anthropic.Client
	timeout time.Duration
	logger  zerolog.Logger
}

func NewMimoClient(apiKey string, timeout time.Duration, logger zerolog.Logger) *MimoClient {
	client := anthropic.NewClient(
		option.WithBaseURL(mimoBaseURL),
		// WithAPIKey satisfies the SDK's own client-side credential-presence
		// check (it sets X-Api-Key too, which MiMo ignores); WithHeader
		// after it is what actually authenticates against MiMo's endpoint,
		// which expects the literal header name "api-key", not "X-Api-Key".
		option.WithAPIKey(apiKey),
		option.WithHeader("api-key", apiKey),
	)
	return &MimoClient{client: client, timeout: timeout, logger: logger}
}

func (m *MimoClient) Summarize(ctx context.Context, chunk domain.DiffChunk) (domain.FileSummary, error) {
	ctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()

	resp, err := m.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     mimoModel,
		MaxTokens: 1024,
		System:    []anthropic.TextBlockParam{{Text: systemPrompt}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(buildPrompt(chunk))),
		},
		Temperature: anthropic.Float(0.2),
	})
	if err != nil {
		return domain.FileSummary{}, fmt.Errorf("mimo summarize %s: %w", chunk.FilePath, err)
	}
	return parseStructuredOutput(resp, chunk)
}

const systemPrompt = `You are a senior backend engineer reviewing a git diff.
Output ONLY valid JSON matching this schema, no prose outside JSON, no markdown code fence:
{"summary": string, "category": "feat|fix|refactor|chore|docs", "concerns": [string]}`

func buildPrompt(chunk domain.DiffChunk) string {
	return fmt.Sprintf("File: %s\nDiff:\n%s", chunk.FilePath, chunk.Content)
}

func parseStructuredOutput(resp *anthropic.Message, chunk domain.DiffChunk) (domain.FileSummary, error) {
	if len(resp.Content) == 0 {
		return domain.FileSummary{}, fmt.Errorf("empty response from mimo for %s", chunk.FilePath)
	}
	text := strings.TrimSpace(resp.Content[0].Text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	var parsed struct {
		Summary  string   `json:"summary"`
		Category string   `json:"category"`
		Concerns []string `json:"concerns"`
	}
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return domain.FileSummary{}, fmt.Errorf("parse mimo output for %s: %w", chunk.FilePath, err)
	}
	return domain.FileSummary{
		FilePath: chunk.FilePath,
		Summary:  parsed.Summary,
		Category: parsed.Category,
		Concerns: parsed.Concerns,
		Risk:     chunk.Risk,
		FromLLM:  true,
	}, nil
}
