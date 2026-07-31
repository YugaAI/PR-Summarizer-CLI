package config

import "testing"

func TestLoad_MissingGitHubToken(t *testing.T) {
	t.Setenv("MIMO_API_KEY", "x")
	t.Setenv("DIFF_BASE_REF", "main")
	t.Setenv("DIFF_HEAD_REF", "feature")
	t.Setenv("GITHUB_REPOSITORY_OWNER", "owner")
	t.Setenv("GITHUB_REPO_NAME", "repo")
	t.Setenv("PR_NUMBER", "1")

	if _, err := Load(); err == nil {
		t.Fatal("expected error when GITHUB_TOKEN is missing, got nil")
	}
}

func TestLoad_AllRequiredPresent(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "token")
	t.Setenv("MIMO_API_KEY", "x")
	t.Setenv("DIFF_BASE_REF", "main")
	t.Setenv("DIFF_HEAD_REF", "feature")
	t.Setenv("GITHUB_REPOSITORY_OWNER", "owner")
	t.Setenv("GITHUB_REPO_NAME", "repo")
	t.Setenv("PR_NUMBER", "1")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.TokenBudget != 8000 {
		t.Errorf("expected default TokenBudget 8000, got %d", cfg.TokenBudget)
	}
	if cfg.Concurrency != 3 {
		t.Errorf("expected default Concurrency 3, got %d", cfg.Concurrency)
	}
}
