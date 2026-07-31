package cache

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/YugaAI/PR-Summarizer-CLI/internal/domain"
)

// LocalCache is a file-based Cache: one JSON file per key under Dir.
type LocalCache struct {
	Dir string
}

func NewLocalCache(dir string) *LocalCache {
	return &LocalCache{Dir: dir}
}

func (c *LocalCache) Get(ctx context.Context, key string) (domain.FileSummary, bool) {
	data, err := os.ReadFile(c.path(key))
	if err != nil {
		return domain.FileSummary{}, false
	}
	var summary domain.FileSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return domain.FileSummary{}, false
	}
	return summary, true
}

func (c *LocalCache) Set(ctx context.Context, key string, value domain.FileSummary) error {
	if err := os.MkdirAll(c.Dir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return os.WriteFile(c.path(key), data, 0o644)
}

func (c *LocalCache) path(key string) string {
	return filepath.Join(c.Dir, key+".json")
}
