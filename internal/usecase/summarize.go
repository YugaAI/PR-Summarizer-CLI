package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"

	"github.com/YugaAI/PR-Summarizer-CLI/internal/chunker"
	"github.com/YugaAI/PR-Summarizer-CLI/internal/classifier"
	"github.com/YugaAI/PR-Summarizer-CLI/internal/domain"
	"github.com/YugaAI/PR-Summarizer-CLI/internal/repository/cache"
	"github.com/YugaAI/PR-Summarizer-CLI/internal/repository/diff"
	"github.com/YugaAI/PR-Summarizer-CLI/internal/repository/llm"
	"github.com/YugaAI/PR-Summarizer-CLI/internal/repository/vcs"
)

type Summarizer struct {
	extractor   diff.DiffExtractor
	classifier  func([]domain.DiffFile, classifier.RiskRules) []domain.DiffFile
	rules       classifier.RiskRules
	chunker     chunker.Chunker
	llm         llm.LLMSummarizer
	cache       cache.Cache
	vcs         vcs.VCSClient
	concurrency int
	logger      zerolog.Logger
}

func NewSummarizer(
	extractor diff.DiffExtractor,
	classifyFn func([]domain.DiffFile, classifier.RiskRules) []domain.DiffFile,
	rules classifier.RiskRules,
	chunk chunker.Chunker,
	llmSummarizer llm.LLMSummarizer,
	cacheImpl cache.Cache,
	vcsClient vcs.VCSClient,
	concurrency int,
	logger zerolog.Logger,
) *Summarizer {
	return &Summarizer{
		extractor:   extractor,
		classifier:  classifyFn,
		rules:       rules,
		chunker:     chunk,
		llm:         llmSummarizer,
		cache:       cacheImpl,
		vcs:         vcsClient,
		concurrency: concurrency,
		logger:      logger,
	}
}

// Run extracts the diff, classifies risk, summarizes each chunk (cache, then
// LLM with heuristic fallback on failure, skipping the LLM entirely for
// low-value files or whitespace-only diffs), and posts the merged result as
// a PR comment.
func (s *Summarizer) Run(ctx context.Context, prNumber int) error {
	files, err := s.extractor.Extract(ctx)
	if err != nil {
		return err
	}

	classified := s.classifier(files, s.rules)
	chunks := s.chunker.Chunk(classified)

	meaningful, err := s.extractor.HasMeaningfulChanges(ctx)
	if err != nil {
		return err
	}

	results := make([]domain.FileSummary, len(chunks))
	if !meaningful {
		s.logger.Info().Msg("whitespace-only diff, skipping LLM entirely")
		for i, c := range chunks {
			results[i] = heuristicFallback(c)
		}
	} else {
		g, gctx := errgroup.WithContext(ctx)
		g.SetLimit(s.concurrency)
		for i, c := range chunks {
			g.Go(func() error {
				results[i] = s.summarizeChunk(gctx, c)
				return nil
			})
		}
		if err := g.Wait(); err != nil {
			return err
		}
	}

	summary := domain.MergeSummaries(results)
	body := summary.RenderMarkdown()

	if err := s.vcs.PostOrUpdateComment(ctx, prNumber, body); err != nil {
		return err
	}

	s.logger.Info().
		Int("high_risk_count", summary.HighRiskCount).
		Int("files", len(results)).
		Int("cache_hits", summary.CacheHits).
		Int("cache_misses", summary.CacheMisses).
		Msg("pr summary posted")
	return nil
}

// summarizeChunk resolves one chunk's summary: skip patterns and cache hits
// bypass the LLM entirely; a failed LLM call falls back to heuristics.
func (s *Summarizer) summarizeChunk(ctx context.Context, c domain.DiffChunk) domain.FileSummary {
	if c.Skip {
		return heuristicFallback(c)
	}

	key := c.Hash()
	if cached, ok := s.cache.Get(ctx, key); ok {
		cached.FromCache = true
		return cached
	}

	summary, err := s.llm.Summarize(ctx, c)
	if err != nil {
		s.logger.Warn().Err(err).Str("file", c.FilePath).Msg("llm summarize failed, using heuristic fallback")
		return heuristicFallback(c)
	}

	if err := s.cache.Set(ctx, key, summary); err != nil {
		s.logger.Warn().Err(err).Str("file", c.FilePath).Msg("cache set failed")
	}
	return summary
}

// heuristicFallback produces a FileSummary without calling the LLM, used
// when the LLM call fails or times out, or the chunk is skipped entirely.
// Category is guessed from a conventional-commit-style prefix ("feat:",
// "fix:", ...) found in the diff content, defaulting to "chore" when none
// is found.
func heuristicFallback(chunk domain.DiffChunk) domain.FileSummary {
	return domain.FileSummary{
		FilePath: chunk.FilePath,
		Summary:  fmt.Sprintf("%s risk change in %s", chunk.Risk, chunk.FilePath),
		Category: guessCategory(chunk.Content),
		Risk:     chunk.Risk,
		FromLLM:  false,
	}
}

var conventionalPrefixes = []string{"feat", "fix", "refactor", "chore", "docs"}

func guessCategory(diffContent string) string {
	lower := strings.ToLower(diffContent)
	for _, p := range conventionalPrefixes {
		if strings.Contains(lower, p+":") {
			return p
		}
	}
	return "chore"
}
