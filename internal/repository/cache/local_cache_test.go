package cache

import (
	"context"
	"os"
	"reflect"
	"testing"

	"github.com/YugaAI/PR-Summarizer-CLI/internal/domain"
)

func TestLocalCache_MissThenSetThenHit(t *testing.T) {
	dir := t.TempDir()
	c := NewLocalCache(dir)
	ctx := context.Background()

	if _, ok := c.Get(ctx, "missing-key"); ok {
		t.Fatal("expected cache miss for key that was never set")
	}

	want := domain.FileSummary{FilePath: "a.go", Summary: "does a thing", Category: "feat", Risk: domain.RiskLow}
	if err := c.Set(ctx, "key1", want); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	got, ok := c.Get(ctx, "key1")
	if !ok {
		t.Fatal("expected cache hit after Set")
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestLocalCache_CorruptFileTreatedAsMiss(t *testing.T) {
	dir := t.TempDir()
	c := NewLocalCache(dir)
	ctx := context.Background()

	if err := c.Set(ctx, "bad", domain.FileSummary{}); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	// Overwrite with invalid JSON to simulate corruption.
	if err := os.WriteFile(c.path("bad"), []byte("not json"), 0o644); err != nil {
		t.Fatalf("failed to corrupt cache file: %v", err)
	}

	if _, ok := c.Get(ctx, "bad"); ok {
		t.Fatal("expected corrupt cache entry to be treated as a miss")
	}
}
