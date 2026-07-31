package domain

import (
	"strconv"
	"strings"
)

type FileSummary struct {
	FilePath  string
	Summary   string
	Category  string // "feat" | "fix" | "refactor" | "chore" | "docs"
	Concerns  []string
	Risk      RiskLevel
	FromCache bool
	FromLLM   bool // false = heuristic fallback dipakai
}

type PRSummary struct {
	Files         []FileSummary
	HighRiskCount int
	TokensUsed    int
	CacheHits     int
	CacheMisses   int
}

func MergeSummaries(results []FileSummary) PRSummary {
	summary := PRSummary{Files: results}
	for _, r := range results {
		if r.Risk == RiskHigh {
			summary.HighRiskCount++
		}
		if r.FromCache {
			summary.CacheHits++
		} else {
			summary.CacheMisses++
		}
	}
	return summary
}

func (s PRSummary) RenderMarkdown() string {
	var b strings.Builder
	b.WriteString("<!-- pr-summarizer-bot -->\n")
	b.WriteString("## PR Summary\n\n")
	b.WriteString("| File | Risk | Summary |\n|---|---|---|\n")
	for _, f := range s.Files {
		summary := f.Summary
		if summary == "" {
			summary = "-"
		}
		b.WriteString("| " + f.FilePath + " | " + string(f.Risk) + " | " + summary + " |\n")
	}
	if s.HighRiskCount > 0 {
		b.WriteString("\n**High risk files:** ")
		b.WriteString(strconv.Itoa(s.HighRiskCount))
		b.WriteString("\n")
	}
	b.WriteString("\n<!-- tokens_used: ")
	b.WriteString(strconv.Itoa(s.TokensUsed))
	b.WriteString(", cache_hit: ")
	b.WriteString(strconv.Itoa(s.CacheHits))
	b.WriteString("/")
	b.WriteString(strconv.Itoa(s.CacheHits + s.CacheMisses))
	b.WriteString(" -->\n")
	return b.String()
}
