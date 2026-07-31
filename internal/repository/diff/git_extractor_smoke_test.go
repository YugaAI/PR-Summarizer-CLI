package diff

import (
	"context"
	"testing"
)

// TestGitExtractor_RealRepoSmoke runs against this repo's own history as a
// sanity check that the three-dot diff and parsing logic behave against real
// git output, not just synthetic fixtures. Temporary — remove once Phase 2
// live validation (plan.md 4.3) is done.
func TestGitExtractor_RealRepoSmoke(t *testing.T) {
	e := NewGitExtractor("875ff24", "b272659")
	files, err := e.Extract(context.Background())
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}
	if len(files) != 11 {
		t.Fatalf("expected 11 changed files, got %d", len(files))
	}
	found := false
	for _, f := range files {
		if f.Path == "plan.md" {
			found = true
			if f.Status != "added" {
				t.Errorf("expected plan.md status 'added', got %q", f.Status)
			}
			if f.Content == "" {
				t.Errorf("expected non-empty diff content for plan.md")
			}
		}
	}
	if !found {
		t.Fatal("expected plan.md in changed files")
	}
}
