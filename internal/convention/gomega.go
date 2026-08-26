package convention

import (
	"go/ast"
	"go/token"
	"strings"
)

// gomegaCtors are the calls that build a gomega instance.
var gomegaCtors = map[string]struct{}{
	"NewWithT":       {},
	"NewGomega":      {},
	"NewGomegaWithT": {},
}

// namedGomega reports an assertion made through a gomega built in the same
// expression.
//
// NewWithT(t).Expect(x) reads as one thing and is two, and the next assertion
// has to either repeat the construction or rewrite the line.
func namedGomega(fset *token.FileSet, files []*ast.File) []Finding {
	var out []Finding
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			inner, ok := sel.X.(*ast.CallExpr)
			if !ok || !isGomegaCtor(inner) {
				return true
			}
			out = append(out, Finding{
				At:    at(fset, call.Pos()),
				Check: "named-gomega",
				What:  "assertion goes through a gomega built in the same expression; give it a name first",
			})
			return true
		})
	}
	return out
}

// isGomegaCtor reports whether a call builds a gomega instance.
func isGomegaCtor(call *ast.CallExpr) bool {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		_, ok := gomegaCtors[fn.Name]
		return ok
	case *ast.SelectorExpr:
		_, ok := gomegaCtors[fn.Sel.Name]
		return ok
	}
	return false
}

// gomegaInSubtest reports a subtest closure asserting through a gomega bound
// outside it.
//
// Binding gomega to the parent t attributes the failure to the function instead
// of the row, hides the row's name, and stops the table at the first failure.
// Creating one inside the closure fixes all three for one line.
//
// What this reports is a closure reaching outward. Binding one outside the loop
// is allowed when it is used out there, which setup running once before the loop
// needs.
func gomegaInSubtest(fset *token.FileSet, files []*ast.File) []Finding {
	var out []Finding
	for _, file := range files {
		outer := gomegaNames(file, nil)
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || !isSubtestCall(call) || len(call.Args) < 2 {
				return true
			}
			body, ok := call.Args[1].(*ast.FuncLit)
			if !ok {
				return true
			}
			inner := gomegaNames(body, nil)
			for _, use := range gomegaUses(body) {
				if _, own := inner[use.name]; own {
					continue
				}
				if _, reached := outer[use.name]; !reached {
					continue
				}
				out = append(out, Finding{
					At:    at(fset, use.pos),
					Check: "gomega-in-subtest",
					What:  "subtest uses " + use.name + " from outside the closure; create the gomega inside it",
				})
			}
			return true
		})
	}
	return out
}

// isSubtestCall reports whether a call is t.Run.
func isSubtestCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "Run"
}

// gomegaNames collects the variables in a node that were assigned a gomega.
func gomegaNames(n ast.Node, into map[string]struct{}) map[string]struct{} {
	if into == nil {
		into = map[string]struct{}{}
	}
	ast.Inspect(n, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for i, rhs := range assign.Rhs {
			call, ok := rhs.(*ast.CallExpr)
			if !ok || !isGomegaCtor(call) || i >= len(assign.Lhs) {
				continue
			}
			if id, ok := assign.Lhs[i].(*ast.Ident); ok {
				into[id.Name] = struct{}{}
			}
		}
		return true
	})
	return into
}

// use is one place a name was used to assert.
type use struct {
	name string
	pos  token.Pos
}

// gomegaUses collects every assertion made through a named identifier.
func gomegaUses(n ast.Node) []use {
	var out []use
	ast.Inspect(n, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || !isAssertion(sel.Sel.Name) {
			return true
		}
		id, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		out = append(out, use{name: id.Name, pos: call.Pos()})
		return true
	})
	return out
}

// isAssertion reports whether a method name is one gomega asserts through.
func isAssertion(name string) bool {
	prefixes := []string{"Expect", "Eventually", "Consistently", "Ω"}
	for _, p := range prefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}
