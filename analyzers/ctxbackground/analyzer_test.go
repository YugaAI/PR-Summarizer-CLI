package ctxbackground

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), Analyzer, "a")
}

func TestAnalyzer_Allowlist(t *testing.T) {
	allow = "legacypkg"
	defer func() { allow = "" }()

	analysistest.Run(t, analysistest.TestData(), Analyzer, "legacypkg")
}
