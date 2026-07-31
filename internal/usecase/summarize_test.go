package usecase

import (
	"context"
	"strings"
	"testing"

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

func TestRun_PostsCommentWithMarkerAndRiskCount(t *testing.T) {
	extractor := &fakeExtractor{files: []domain.DiffFile{
		{Path: "internal/auth/handler/login.go"},
		{Path: "README.md"},
	}}
	rules := classifier.RiskRules{
		High: []classifier.Rule{{Pattern: "**/auth/**", Reason: "auth logic"}},
	}
	vcsClient := &fakeVCS{}

	s := NewSummarizer(extractor, classifier.Classify, rules, chunker.NewFileChunker(), vcsClient, zerolog.Nop())

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
}
