// Package ctxbackground defines an analyzer that flags context.Background()
// calls made outside func main(), since request-scoped code that starts a
// fresh background context silently drops the caller's cancellation and
// deadline (plan.md §2.1: "operasi tetap jalan meski client cancel/timeout").
package ctxbackground

import (
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// allow is a comma-separated list of path substrings exempted from this
// check, configured via `-ctxbackground.allow=...` when run through
// `go vet -vettool=`.
var allow string

var Analyzer = &analysis.Analyzer{
	Name:     "ctxbackground",
	Doc:      "reports context.Background() calls outside func main(), which silently drop the caller's cancellation/deadline",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

func init() {
	Analyzer.Flags.StringVar(&allow, "allow", "", "comma-separated list of file path substrings to exempt from this check")
}

func run(pass *analysis.Pass) (any, error) {
	patterns := splitAllowlist(allow)
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	insp.Preorder([]ast.Node{(*ast.FuncDecl)(nil)}, func(n ast.Node) {
		fn := n.(*ast.FuncDecl)
		if fn.Name.Name == "main" || fn.Body == nil {
			return
		}

		filename := pass.Fset.Position(fn.Pos()).Filename
		// Test files legitimately construct a fresh context.Background()
		// all the time (there's no incoming request to propagate), so they
		// are exempt by default rather than requiring every test file to be
		// listed in -allow.
		if strings.HasSuffix(filename, "_test.go") || matchesAny(filename, patterns) {
			return
		}

		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || !isContextBackgroundCall(call) {
				return true
			}
			pass.Reportf(call.Pos(), "context.Background() used outside main(); propagate the caller's context instead")
			return true
		})
	})

	return nil, nil
}

func isContextBackgroundCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Background" {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	return ok && ident.Name == "context"
}

func splitAllowlist(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func matchesAny(path string, patterns []string) bool {
	for _, p := range patterns {
		if p != "" && strings.Contains(path, p) {
			return true
		}
	}
	return false
}
