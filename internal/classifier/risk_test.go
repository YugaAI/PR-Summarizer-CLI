package classifier

import (
	"testing"

	"github.com/YugaAI/PR-Summarizer-CLI/internal/domain"
)

func testRules() RiskRules {
	return RiskRules{
		High: []Rule{
			{Pattern: "**/auth/**", Reason: "authentication/authorization logic"},
			{Pattern: "**/migrations/**", Reason: "database schema change"},
		},
		Medium: []Rule{
			{Pattern: "go.mod", Reason: "dependency change"},
		},
		Low: []Rule{
			{Pattern: "**/*_test.go", Reason: "test file"},
			{Pattern: "docs/**", Reason: "documentation"},
		},
	}
}

func TestClassify(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		wantRisk domain.RiskLevel
	}{
		{"matches high rule", "internal/migrations/0001_init.sql", domain.RiskHigh},
		{"matches low rule", "docs/readme.md", domain.RiskLow},
		{"no matching rule defaults to medium", "internal/usecase/summarize.go", domain.RiskMedium},
		{"nested wildcard matches high auth rule", "internal/auth/handler/login.go", domain.RiskHigh},
	}

	rules := testRules()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := []domain.DiffFile{{Path: tt.path}}
			got := Classify(files, rules)
			if len(got) != 1 {
				t.Fatalf("expected 1 result, got %d", len(got))
			}
			if got[0].Risk != tt.wantRisk {
				t.Errorf("path %q: got risk %q, want %q", tt.path, got[0].Risk, tt.wantRisk)
			}
			if got[0].RiskReason == "" {
				t.Errorf("path %q: expected non-empty RiskReason", tt.path)
			}
		})
	}
}
