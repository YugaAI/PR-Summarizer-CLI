package llm

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/YugaAI/PR-Summarizer-CLI/internal/domain"
)

func TestParseStructuredOutput_PlainJSON(t *testing.T) {
	resp := &anthropic.Message{Content: []anthropic.ContentBlockUnion{
		{Type: "text", Text: `{"summary": "adds login handler", "category": "feat", "concerns": ["no rate limiting"]}`},
	}}
	chunk := domain.DiffChunk{FilePath: "auth/login.go", Risk: domain.RiskHigh}

	got, err := parseStructuredOutput(resp, chunk)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Summary != "adds login handler" || got.Category != "feat" || len(got.Concerns) != 1 {
		t.Errorf("unexpected result: %+v", got)
	}
	if !got.FromLLM {
		t.Error("expected FromLLM to be true")
	}
	if got.Risk != domain.RiskHigh {
		t.Errorf("expected risk carried over from chunk, got %q", got.Risk)
	}
}

func TestParseStructuredOutput_StripsMarkdownFence(t *testing.T) {
	resp := &anthropic.Message{Content: []anthropic.ContentBlockUnion{
		{Type: "text", Text: "```json\n{\"summary\": \"fixes off-by-one\", \"category\": \"fix\", \"concerns\": []}\n```"},
	}}
	chunk := domain.DiffChunk{FilePath: "util/math.go"}

	got, err := parseStructuredOutput(resp, chunk)
	if err != nil {
		t.Fatalf("unexpected error parsing fenced response: %v", err)
	}
	if got.Summary != "fixes off-by-one" || got.Category != "fix" {
		t.Errorf("unexpected result: %+v", got)
	}
}

func TestParseStructuredOutput_EmptyContent(t *testing.T) {
	resp := &anthropic.Message{Content: nil}
	chunk := domain.DiffChunk{FilePath: "x.go"}

	if _, err := parseStructuredOutput(resp, chunk); err == nil {
		t.Fatal("expected error for empty content, got nil")
	}
}

func TestParseStructuredOutput_InvalidJSON(t *testing.T) {
	resp := &anthropic.Message{Content: []anthropic.ContentBlockUnion{
		{Type: "text", Text: "not json at all"},
	}}
	chunk := domain.DiffChunk{FilePath: "x.go"}

	if _, err := parseStructuredOutput(resp, chunk); err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}
