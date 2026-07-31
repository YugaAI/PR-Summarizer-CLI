// Command mimo-smoke-test is a throwaway manual validation tool for plan.md
// Task Validasi Wajib 5.3: confirm the "api-key" header actually overrides
// the Anthropic SDK's default auth header against the real MiMo endpoint,
// and that responses aren't wrapped in a markdown code fence in practice.
// Delete this once that validation is done and recorded in the Decision Log.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/YugaAI/PR-Summarizer-CLI/internal/domain"
	"github.com/YugaAI/PR-Summarizer-CLI/internal/repository/llm"
	"github.com/rs/zerolog"
)

func main() {
	apiKey := os.Getenv("MIMO_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "MIMO_API_KEY is required")
		os.Exit(1)
	}

	client := llm.NewMimoClient(apiKey, 30*time.Second, zerolog.New(os.Stderr).With().Timestamp().Logger())

	chunk := domain.DiffChunk{
		FilePath: "internal/example/hello.go",
		Content:  "+func Hello() string {\n+\treturn \"hello\"\n+}\n",
		Risk:     domain.RiskLow,
	}

	summary, err := client.Summarize(context.Background(), chunk)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("OK: summary=%q category=%q concerns=%v\n", summary.Summary, summary.Category, summary.Concerns)
}
