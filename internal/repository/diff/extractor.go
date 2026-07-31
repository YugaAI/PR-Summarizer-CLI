package diff

import (
	"context"

	"github.com/YugaAI/PR-Summarizer-CLI/internal/domain"
)

type DiffExtractor interface {
	Extract(ctx context.Context) ([]domain.DiffFile, error)
	// HasMeaningfulChanges reports whether the diff has any non-whitespace
	// changes. false means every changed line differs only in whitespace,
	// so the LLM layer should be skipped entirely for the PR.
	HasMeaningfulChanges(ctx context.Context) (bool, error)
}
