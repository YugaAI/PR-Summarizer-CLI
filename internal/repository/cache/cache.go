package cache

import (
	"context"

	"github.com/YugaAI/PR-Summarizer-CLI/internal/domain"
)

type Cache interface {
	Get(ctx context.Context, key string) (domain.FileSummary, bool)
	Set(ctx context.Context, key string, value domain.FileSummary) error
}
