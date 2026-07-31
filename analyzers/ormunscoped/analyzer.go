// Package ormunscoped defines an analyzer that flags .Save() calls with no
// .Where() earlier in the same method chain, since an unscoped GORM Save can
// update columns/rows that weren't intended, especially on messy production
// data (plan.md §2.1).
//
// This is a purely syntactic, name-based heuristic (no type-checking against
// gorm.io/gorm) — it will also flag any unrelated type that happens to
// expose a .Save() method. That's a known false-positive source, which is
// exactly why plan.md's Definition of Done requires running this against a
// real codebase and reviewing every finding before it's wired up as a
// required check.
package ormunscoped

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

var Analyzer = &analysis.Analyzer{
	Name:     "ormunscoped",
	Doc:      "reports .Save() calls with no .Where() earlier in the same chain, which can update unintended rows",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	insp.Preorder([]ast.Node{(*ast.CallExpr)(nil)}, func(n ast.Node) {
		call := n.(*ast.CallExpr)
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Save" {
			return
		}
		if chainHasWhere(sel.X) {
			return
		}
		pass.Reportf(call.Pos(), "unscoped .Save() call: no .Where() earlier in the chain, this can update unintended rows")
	})

	return nil, nil
}

// chainHasWhere walks back through a method-chain expression, e.g.
// db.Model(&User{}).Where(...).Save(u), looking for a .Where() call
// anywhere earlier in the chain.
func chainHasWhere(expr ast.Expr) bool {
	for {
		call, ok := expr.(*ast.CallExpr)
		if !ok {
			return false
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		if sel.Sel.Name == "Where" {
			return true
		}
		expr = sel.X
	}
}
