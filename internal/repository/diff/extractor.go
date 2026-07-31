package diff

import (
	"context"

	"github.com/YugaAI/PR-Summarizer-CLI/internal/domain"
)

type DiffExtractor interface {
	Extract(ctx context.Context) ([]domain.DiffFile, error)
}
