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
	files []domain.DiffFile
}

func (f *fakeExtractor) Extract(ctx context.Context) ([]domain.DiffFile, error) {
	return f.files, nil
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

func TestRun_PostsCommentWithMarkerAndRiskCount(t *testing.T) {
	extractor := &fakeExtractor{files: []domain.DiffFile{
		{Path: "internal/auth/handler/login.go"},
		{Path: "README.md"},
	}}
	rules := classifier.RiskRules{
		High: []classifier.Rule{{Pattern: "**/auth/**", Reason: "auth logic"}},
	}
	vcsClient := &fakeVCS{}
	llmClient := &fakeLLM{}

	s := NewSummarizer(extractor, classifier.Classify, rules, chunker.NewFileChunker(), llmClient, vcsClient, 3, zerolog.Nop())

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
	extractor := &fakeExtractor{files: []domain.DiffFile{
		{Path: "flaky.go"},
	}}
	vcsClient := &fakeVCS{}
	llmClient := &fakeLLM{failPaths: map[string]bool{"flaky.go": true}}

	s := NewSummarizer(extractor, classifier.Classify, classifier.RiskRules{}, chunker.NewFileChunker(), llmClient, vcsClient, 3, zerolog.Nop())

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
	extractor := &fakeExtractor{files: files}
	vcsClient := &fakeVCS{}
	llmClient := &concurrencyTrackingLLM{}
	const limit = 3

	s := NewSummarizer(extractor, classifier.Classify, classifier.RiskRules{}, chunker.NewFileChunker(), llmClient, vcsClient, limit, zerolog.Nop())

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
