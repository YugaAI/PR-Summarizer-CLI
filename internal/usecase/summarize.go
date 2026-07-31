package usecase

import (
	"context"

	"github.com/rs/zerolog"

	"github.com/YugaAI/PR-Summarizer-CLI/internal/chunker"
	"github.com/YugaAI/PR-Summarizer-CLI/internal/classifier"
	"github.com/YugaAI/PR-Summarizer-CLI/internal/domain"
	"github.com/YugaAI/PR-Summarizer-CLI/internal/repository/diff"
	"github.com/YugaAI/PR-Summarizer-CLI/internal/repository/vcs"
)

type Summarizer struct {
	extractor  diff.DiffExtractor
	classifier func([]domain.DiffFile, classifier.RiskRules) []domain.DiffFile
	rules      classifier.RiskRules
	chunker    chunker.Chunker
	vcs        vcs.VCSClient
	logger     zerolog.Logger
}

func NewSummarizer(
	extractor diff.DiffExtractor,
	classifyFn func([]domain.DiffFile, classifier.RiskRules) []domain.DiffFile,
	rules classifier.RiskRules,
	chunk chunker.Chunker,
	vcsClient vcs.VCSClient,
	logger zerolog.Logger,
) *Summarizer {
	return &Summarizer{
		extractor:  extractor,
		classifier: classifyFn,
		rules:      rules,
		chunker:    chunk,
		vcs:        vcsClient,
		logger:     logger,
	}
}

// Run extracts the diff, classifies risk, and posts a heuristic-only summary
// comment. LLM narration is layered on top in Phase 3 without changing this
// flow's shape.
func (s *Summarizer) Run(ctx context.Context, prNumber int) error {
	files, err := s.extractor.Extract(ctx)
	if err != nil {
		return err
	}

	classified := s.classifier(files, s.rules)
	chunks := s.chunker.Chunk(classified)

	results := make([]domain.FileSummary, len(chunks))
	for i, c := range chunks {
		results[i] = domain.FileSummary{
			FilePath: c.FilePath,
			Risk:     c.Risk,
			FromLLM:  false,
		}
	}

	summary := domain.MergeSummaries(results)
	body := summary.RenderMarkdown()

	if err := s.vcs.PostOrUpdateComment(ctx, prNumber, body); err != nil {
		return err
	}

	s.logger.Info().Int("high_risk_count", summary.HighRiskCount).Int("files", len(results)).Msg("pr summary posted")
	return nil
}
