// Command precommit-guard is a standalone multichecker binary bundling the
// project's custom analyzers, meant to be run either directly or as a
// `go vet -vettool=` plugin.
package main

import (
	"golang.org/x/tools/go/analysis/multichecker"

	"github.com/YugaAI/PR-Summarizer-CLI/analyzers/ctxbackground"
	"github.com/YugaAI/PR-Summarizer-CLI/analyzers/ormunscoped"
)

func main() {
	multichecker.Main(
		ctxbackground.Analyzer,
		ormunscoped.Analyzer,
	)
}
