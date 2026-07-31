package chunker

import (
	"os"

	"github.com/bmatcuk/doublestar/v4"
	"gopkg.in/yaml.v3"

	"github.com/YugaAI/PR-Summarizer-CLI/internal/domain"
)

type Chunker interface {
	Chunk(files []domain.DiffFile) []domain.DiffChunk
}

// FileChunker maps each file to exactly one chunk (1:1). Token-budget-aware
// chunking is deferred to a later phase. Files matching skipPatterns are
// marked Skip so the usecase can bypass the LLM call entirely for low-value
// content (lockfiles, generated code, vendor).
type FileChunker struct {
	skipPatterns []string
}

func NewFileChunker(skipPatterns []string) *FileChunker {
	return &FileChunker{skipPatterns: skipPatterns}
}

func (c *FileChunker) Chunk(files []domain.DiffFile) []domain.DiffChunk {
	chunks := make([]domain.DiffChunk, len(files))
	for i, f := range files {
		chunks[i] = domain.DiffChunk{
			FilePath: f.Path,
			Content:  f.Content,
			Risk:     f.Risk,
			Skip:     matchesAny(f.Path, c.skipPatterns),
		}
	}
	return chunks
}

func matchesAny(path string, patterns []string) bool {
	for _, p := range patterns {
		if ok, _ := doublestar.Match(p, path); ok {
			return true
		}
	}
	return false
}

type skipPatternsFile struct {
	Patterns []string `yaml:"patterns"`
}

func LoadSkipPatterns(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var parsed skipPatternsFile
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}
	return parsed.Patterns, nil
}
