package chunker

import (
	"testing"

	"github.com/YugaAI/PR-Summarizer-CLI/internal/domain"
)

func TestFileChunker_Chunk_Empty(t *testing.T) {
	got := NewFileChunker().Chunk(nil)
	if len(got) != 0 {
		t.Fatalf("expected 0 chunks for empty input, got %d", len(got))
	}
}

func TestFileChunker_Chunk_OneToOne(t *testing.T) {
	files := []domain.DiffFile{
		{Path: "a.go", Content: "diff a", Risk: domain.RiskHigh},
		{Path: "b.go", Content: "diff b", Risk: domain.RiskLow},
	}

	got := NewFileChunker().Chunk(files)

	if len(got) != len(files) {
		t.Fatalf("expected %d chunks, got %d", len(files), len(got))
	}
	for i, f := range files {
		if got[i].FilePath != f.Path || got[i].Content != f.Content || got[i].Risk != f.Risk {
			t.Errorf("chunk %d = %+v, want derived from %+v", i, got[i], f)
		}
	}
}

func TestFileChunker_Chunk_GiantSingleFile(t *testing.T) {
	huge := make([]byte, 5*1024*1024)
	for i := range huge {
		huge[i] = 'x'
	}
	files := []domain.DiffFile{{Path: "giant.go", Content: string(huge), Risk: domain.RiskMedium}}

	got := NewFileChunker().Chunk(files)

	if len(got) != 1 || len(got[0].Content) != len(huge) {
		t.Fatalf("expected single chunk preserving full content, got %d chunks", len(got))
	}
}
