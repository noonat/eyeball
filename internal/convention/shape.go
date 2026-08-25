package convention

import (
	"go/ast"
	"go/token"
)

// braceLines reports a declared function whose braces sit on one line.
//
// A one-line body reads as a value rather than as code, so the next person adds
// a statement and reformats the whole thing, and the diff hides what changed.
//
// A function literal is exempt. A small transform passed as an argument is the
// one place the compact form is clearer, and this only walks declarations.
func braceLines(fset *token.FileSet, files []*ast.File) []Finding {
	var out []Finding
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			open := fset.Position(fn.Body.Lbrace).Line
			if open != fset.Position(fn.Body.Rbrace).Line {
				continue
			}
			out = append(out, Finding{
				At:    at(fset, fn.Pos()),
				Check: "brace-lines",
				What: "func " + fn.Name.Name +
					" opens and closes its braces on one line",
			})
		}
	}
	return out
}

// argumentWrapping reports an argument list that wraps some of the way.
//
// If a newline falls between two arguments then every argument goes on its own
// line and the first break is after the open paren. The half-wrapped form is
// what this rules out: the call's name and an argument share a line, so a reader
// has to find where the list starts, and adding an argument reflows the call.
//
// A newline inside one argument does not count, which keeps a keyed struct
// literal passed to append legal.
func argumentWrapping(fset *token.FileSet, files []*ast.File) []Finding {
	var out []Finding
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) < 2 {
				return true
			}
			out = append(out, wrapFindings(fset, call)...)
			return true
		})
	}
	return out
}

// wrapFindings reports what is wrong with one call's argument list, if anything.
func wrapFindings(fset *token.FileSet, call *ast.CallExpr) []Finding {
	line := func(p token.Pos) int { return fset.Position(p).Line }

	wrapped := false
	for i := 1; i < len(call.Args); i++ {
		if line(call.Args[i-1].End()) != line(call.Args[i].Pos()) {
			wrapped = true
			break
		}
	}
	if !wrapped {
		return nil
	}

	var out []Finding
	if line(call.Lparen) == line(call.Args[0].Pos()) {
		out = append(out, Finding{
			At:    at(fset, call.Lparen),
			Check: "argument-wrapping",
			What: "argument list wraps: break after the open paren so the " +
				"first argument gets its own line",
		})
	}
	for i := 1; i < len(call.Args); i++ {
		if line(call.Args[i-1].End()) == line(call.Args[i].Pos()) {
			out = append(out, Finding{
				At:    at(fset, call.Args[i].Pos()),
				Check: "argument-wrapping",
				What: "argument list wraps all or nothing: this argument " +
					"shares a line with the one before it",
			})
		}
	}
	return out
}
