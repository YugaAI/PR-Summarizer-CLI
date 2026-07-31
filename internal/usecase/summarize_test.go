package usecase

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/YugaAI/PR-Summarizer-CLI/internal/chunker"
	"github.com/YugaAI/PR-Summarizer-CLI/internal/classifier"
	"github.com/YugaAI/PR-Summarizer-CLI/internal/domain"
)

type fakeExtractor struct {
	files      []domain.DiffFile
	meaningful bool
}

func (f *fakeExtractor) Extract(ctx context.Context) ([]domain.DiffFile, error) {
	return f.files, nil
}

func (f *fakeExtractor) HasMeaningfulChanges(ctx context.Context) (bool, error) {
	return f.meaningful, nil
}

type fakeVCS struct {
	body    string
	prNum   int
	callCnt int
}

func (f *fakeVCS) PostOrUpdateComment(ctx context.Context, prNumber int, body string) error {
	f.body = body
	f.prNum = prNumber
	f.callCnt++
	return nil
}

// fakeLLM returns a canned success summary, unless the file path is in
// failPaths, in which case it returns an error to trigger the heuristic
// fallback path.
type fakeLLM struct {
	failPaths map[string]bool
	calls     int32
}

func (f *fakeLLM) Summarize(ctx context.Context, chunk domain.DiffChunk) (domain.FileSummary, error) {
	atomic.AddInt32(&f.calls, 1)
	if f.failPaths[chunk.FilePath] {
		return domain.FileSummary{}, errors.New("simulated llm failure")
	}
	return domain.FileSummary{
		FilePath: chunk.FilePath,
		Summary:  "llm narrative for " + chunk.FilePath,
		Category: "feat",
		Risk:     chunk.Risk,
		FromLLM:  true,
	}, nil
}

type fakeCache struct {
	mu   sync.Mutex
	data map[string]domain.FileSummary
}

func newFakeCache() *fakeCache {
	return &fakeCache{data: make(map[string]domain.FileSummary)}
}

func (f *fakeCache) Get(ctx context.Context, key string) (domain.FileSummary, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.data[key]
	return v, ok
}

func (f *fakeCache) Set(ctx context.Context, key string, value domain.FileSummary) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data[key] = value
	return nil
}

func TestRun_PostsCommentWithMarkerAndRiskCount(t *testing.T) {
	extractor := &fakeExtractor{meaningful: true, files: []domain.DiffFile{
		{Path: "internal/auth/handler/login.go"},
		{Path: "README.md"},
	}}
	rules := classifier.RiskRules{
		High: []classifier.Rule{{Pattern: "**/auth/**", Reason: "auth logic"}},
	}
	vcsClient := &fakeVCS{}
	llmClient := &fakeLLM{}

	s := NewSummarizer(extractor, classifier.Classify, rules, chunker.NewFileChunker(nil), llmClient, newFakeCache(), vcsClient, 3, zerolog.Nop())

	if err := s.Run(context.Background(), 42); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if vcsClient.callCnt != 1 {
		t.Fatalf("expected exactly 1 comment call, got %d", vcsClient.callCnt)
	}
	if vcsClient.prNum != 42 {
		t.Errorf("expected PR number 42, got %d", vcsClient.prNum)
	}
	if !strings.HasPrefix(vcsClient.body, "<!-- pr-summarizer-bot -->") {
		t.Error("expected comment body to start with marker")
	}
	if !strings.Contains(vcsClient.body, "internal/auth/handler/login.go | high") {
		t.Error("expected comment body to tag auth file as high risk")
	}
	if !strings.Contains(vcsClient.body, "**High risk files:** 1") {
		t.Error("expected comment body to report HighRiskCount of 1")
	}
	if !strings.Contains(vcsClient.body, "llm narrative for internal/auth/handler/login.go") {
		t.Error("expected comment body to contain LLM narrative")
	}
}

func TestRun_FallsBackToHeuristicWhenLLMFails(t *testing.T) {
	extractor := &fakeExtractor{meaningful: true, files: []domain.DiffFile{
		{Path: "flaky.go"},
	}}
	vcsClient := &fakeVCS{}
	llmClient := &fakeLLM{failPaths: map[string]bool{"flaky.go": true}}

	s := NewSummarizer(extractor, classifier.Classify, classifier.RiskRules{}, chunker.NewFileChunker(nil), llmClient, newFakeCache(), vcsClient, 3, zerolog.Nop())

	if err := s.Run(context.Background(), 1); err != nil {
		t.Fatalf("Run should not fail when LLM errors (fallback expected): %v", err)
	}

	if vcsClient.callCnt != 1 {
		t.Fatalf("expected comment to still be posted via fallback, got %d calls", vcsClient.callCnt)
	}
	if !strings.Contains(vcsClient.body, "risk change in flaky.go") {
		t.Errorf("expected heuristic fallback text in body, got: %s", vcsClient.body)
	}
}

// concurrencyTrackingLLM records the maximum number of Summarize calls that
// were in flight simultaneously, so the concurrency ceiling can be asserted
// deterministically instead of by parsing log timestamps.
type concurrencyTrackingLLM struct {
	mu      sync.Mutex
	current int
	maxSeen int
}

func (f *concurrencyTrackingLLM) Summarize(ctx context.Context, chunk domain.DiffChunk) (domain.FileSummary, error) {
	f.mu.Lock()
	f.current++
	if f.current > f.maxSeen {
		f.maxSeen = f.current
	}
	f.mu.Unlock()

	time.Sleep(20 * time.Millisecond)

	f.mu.Lock()
	f.current--
	f.mu.Unlock()

	return domain.FileSummary{FilePath: chunk.FilePath, FromLLM: true}, nil
}

func TestRun_RespectsConcurrencyLimit(t *testing.T) {
	files := make([]domain.DiffFile, 20)
	for i := range files {
		files[i] = domain.DiffFile{Path: "file.go"}
	}
	extractor := &fakeExtractor{meaningful: true, files: files}
	vcsClient := &fakeVCS{}
	llmClient := &concurrencyTrackingLLM{}
	const limit = 3

	s := NewSummarizer(extractor, classifier.Classify, classifier.RiskRules{}, chunker.NewFileChunker(nil), llmClient, newFakeCache(), vcsClient, limit, zerolog.Nop())

	if err := s.Run(context.Background(), 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	llmClient.mu.Lock()
	maxSeen := llmClient.maxSeen
	llmClient.mu.Unlock()

	if maxSeen > limit {
		t.Errorf("expected at most %d concurrent LLM calls, saw %d", limit, maxSeen)
	}
	if maxSeen < limit {
		t.Errorf("expected concurrency to reach the limit of %d with 20 chunks, only saw %d in flight", limit, maxSeen)
	}
}

func TestRun_CacheHitSkipsLLMCall(t *testing.T) {
	extractor := &fakeExtractor{meaningful: true, files: []domain.DiffFile{
		{Path: "cached.go", Content: "same as last push"},
	}}
	vcsClient := &fakeVCS{}
	llmClient := &fakeLLM{}
	cacheImpl := newFakeCache()

	chunk := domain.DiffChunk{FilePath: "cached.go", Content: "same as last push"}
	cacheImpl.data[chunk.Hash()] = domain.FileSummary{
		FilePath: "cached.go",
		Summary:  "cached narrative",
		Category: "feat",
		FromLLM:  true,
	}

	s := NewSummarizer(extractor, classifier.Classify, classifier.RiskRules{}, chunker.NewFileChunker(nil), llmClient, cacheImpl, vcsClient, 3, zerolog.Nop())

	if err := s.Run(context.Background(), 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if llmClient.calls != 0 {
		t.Errorf("expected 0 LLM calls on cache hit, got %d", llmClient.calls)
	}
	if !strings.Contains(vcsClient.body, "cached narrative") {
		t.Errorf("expected cached summary in comment body, got: %s", vcsClient.body)
	}
}

func TestRun_SkipPatternBypassesLLMCall(t *testing.T) {
	extractor := &fakeExtractor{meaningful: true, files: []domain.DiffFile{
		{Path: "go.sum", Content: "lockfile churn"},
	}}
	vcsClient := &fakeVCS{}
	llmClient := &fakeLLM{}

	s := NewSummarizer(extractor, classifier.Classify, classifier.RiskRules{}, chunker.NewFileChunker([]string{"**/go.sum"}), llmClient, newFakeCache(), vcsClient, 3, zerolog.Nop())

	if err := s.Run(context.Background(), 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if llmClient.calls != 0 {
		t.Errorf("expected 0 LLM calls for skip-pattern file, got %d", llmClient.calls)
	}
	if !strings.Contains(vcsClient.body, "risk change in go.sum") {
		t.Errorf("expected heuristic summary for skipped file, got: %s", vcsClient.body)
	}
}

func TestRun_WhitespaceOnlyDiffSkipsAllLLMCalls(t *testing.T) {
	extractor := &fakeExtractor{meaningful: false, files: []domain.DiffFile{
		{Path: "a.go"},
		{Path: "b.go"},
	}}
	vcsClient := &fakeVCS{}
	llmClient := &fakeLLM{}

	s := NewSummarizer(extractor, classifier.Classify, classifier.RiskRules{}, chunker.NewFileChunker(nil), llmClient, newFakeCache(), vcsClient, 3, zerolog.Nop())

	if err := s.Run(context.Background(), 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if llmClient.calls != 0 {
		t.Errorf("expected 0 LLM calls for whitespace-only diff, got %d", llmClient.calls)
	}
	if vcsClient.callCnt != 1 {
		t.Fatalf("expected comment to still be posted, got %d calls", vcsClient.callCnt)
	}
}
