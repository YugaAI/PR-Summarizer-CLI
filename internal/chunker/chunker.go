package chunker

import "github.com/YugaAI/PR-Summarizer-CLI/internal/domain"

type Chunker interface {
	Chunk(files []domain.DiffFile) []domain.DiffChunk
}

// FileChunker maps each file to exactly one chunk (1:1). Token-budget-aware
// chunking is deferred to Phase 4.
type FileChunker struct{}

func NewFileChunker() *FileChunker {
	return &FileChunker{}
}

func (c *FileChunker) Chunk(files []domain.DiffFile) []domain.DiffChunk {
	chunks := make([]domain.DiffChunk, len(files))
	for i, f := range files {
		chunks[i] = domain.DiffChunk{
			FilePath: f.Path,
			Content:  f.Content,
			Risk:     f.Risk,
		}
	}
	return chunks
}
