package domain

import (
	"crypto/sha256"
	"encoding/hex"
)

type RiskLevel string

const (
	RiskHigh   RiskLevel = "high"
	RiskMedium RiskLevel = "medium"
	RiskLow    RiskLevel = "low"
)

type DiffFile struct {
	Path       string
	Status     string // "added" | "modified" | "deleted" | "renamed"
	Content    string // unified diff, hunk-level
	Risk       RiskLevel
	RiskReason string
}

type DiffChunk struct {
	FilePath string
	Content  string
	Risk     RiskLevel
}

func (c DiffChunk) Hash() string {
	sum := sha256.Sum256([]byte(c.FilePath + c.Content))
	return hex.EncodeToString(sum[:])
}
