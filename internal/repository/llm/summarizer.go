package llm

//go:generate go tool mockgen -source=summarizer.go -destination=mock_summarizer.go -package=llm

import (
	"context"

	"github.com/YugaAI/PR-Summarizer-CLI/internal/domain"
)

type LLMSummarizer interface {
	Summarize(ctx context.Context, chunk domain.DiffChunk) (domain.FileSummary, error)
}
