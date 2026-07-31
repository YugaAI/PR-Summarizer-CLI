package diff

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/YugaAI/PR-Summarizer-CLI/internal/domain"
)

// GitExtractor extracts changed files between BaseRef and HeadRef using a
// three-dot diff (from merge-base), matching the CI checkout semantics.
type GitExtractor struct {
	BaseRef string
	HeadRef string
}

func NewGitExtractor(baseRef, headRef string) *GitExtractor {
	return &GitExtractor{BaseRef: baseRef, HeadRef: headRef}
}

func (g *GitExtractor) Extract(ctx context.Context) ([]domain.DiffFile, error) {
	rangeSpec := g.BaseRef + "..." + g.HeadRef

	nameStatusOut, err := runGit(ctx, "diff", rangeSpec, "--name-status")
	if err != nil {
		return nil, fmt.Errorf("git diff --name-status: %w", err)
	}

	fullDiffOut, err := runGit(ctx, "diff", rangeSpec)
	if err != nil {
		return nil, fmt.Errorf("git diff: %w", err)
	}

	statusByPath := parseNameStatus(nameStatusOut)
	hunksByPath := splitByFile(fullDiffOut)

	files := make([]domain.DiffFile, 0, len(statusByPath))
	for path, status := range statusByPath {
		files = append(files, domain.DiffFile{
			Path:    path,
			Status:  status,
			Content: hunksByPath[path],
		})
	}
	return files, nil
}

// HasMeaningfulChanges reports whether `git diff -w` (ignoring whitespace)
// between BaseRef and HeadRef produces any output.
func (g *GitExtractor) HasMeaningfulChanges(ctx context.Context) (bool, error) {
	rangeSpec := g.BaseRef + "..." + g.HeadRef
	out, err := runGit(ctx, "diff", "-w", rangeSpec)
	if err != nil {
		return false, fmt.Errorf("git diff -w: %w", err)
	}
	return strings.TrimSpace(out) != "", nil
}

func runGit(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s: %w", out.String(), err)
	}
	return out.String(), nil
}

// parseNameStatus turns `git diff --name-status` output into path -> status
// ("added" | "modified" | "deleted" | "renamed"). Renames ("R100\told\tnew")
// are keyed by the new path.
func parseNameStatus(output string) map[string]string {
	result := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 2 {
			continue
		}
		result[fields[len(fields)-1]] = statusFromCode(fields[0])
	}
	return result
}

func statusFromCode(code string) string {
	switch code[0] {
	case 'A':
		return "added"
	case 'D':
		return "deleted"
	case 'R':
		return "renamed"
	default:
		return "modified"
	}
}

// splitByFile splits a unified diff for multiple files into per-file hunks,
// keyed by the "b/" (post-image) path.
func splitByFile(fullDiff string) map[string]string {
	result := make(map[string]string)
	if fullDiff == "" {
		return result
	}

	var currentPath string
	var current strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(fullDiff))
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "diff --git ") {
			if currentPath != "" {
				result[currentPath] = current.String()
			}
			current.Reset()
			currentPath = extractPathFromDiffLine(line)
		}
		current.WriteString(line)
		current.WriteString("\n")
	}
	if currentPath != "" {
		result[currentPath] = current.String()
	}
	return result
}

func extractPathFromDiffLine(line string) string {
	// "diff --git a/old/path b/new/path"
	if _, after, ok := strings.Cut(line, " b/"); ok {
		return after
	}
	return ""
}
