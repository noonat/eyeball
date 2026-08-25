package convention

import (
	"go/ast"
	"go/token"
)

// rangeLiteral reports a range over a composite literal written in place.
//
// The values otherwise sit between range and the loop body, so reading the loop
// means reading past them, and the loop's subject has no name to refer to. This
// holds for every literal, not only a table: a short list of strings has no
// fields to name and is a violation all the same, because the rule is about the
// missing name.
func rangeLiteral(fset *token.FileSet, files []*ast.File) []Finding {
	var out []Finding
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			r, ok := n.(*ast.RangeStmt)
			if !ok {
				return true
			}
			if _, ok := r.X.(*ast.CompositeLit); !ok {
				return true
			}
			out = append(out, Finding{
				At:    at(fset, r.X.Pos()),
				Check: "range-literal",
				What: "range over an anonymous literal; " +
					"assign it to a variable first",
			})
			return true
		})
	}
	return out
}

// keyedRows reports a table row whose fields are positional.
//
// Positional fields stop being readable past two of them, and adding a field
// silently reassigns every existing value in every row.
//
// Only a literal holding struct literals is examined, which is what a table is.
// A slice of strings or of calls is not a table and has no fields to name.
func keyedRows(fset *token.FileSet, files []*ast.File) []Finding {
	var out []Finding
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			outer, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			for _, elt := range outer.Elts {
				row, ok := elt.(*ast.CompositeLit)
				if !ok || len(row.Elts) == 0 {
					continue
				}
				if _, keyed := row.Elts[0].(*ast.KeyValueExpr); keyed {
					continue
				}
				if !looksLikeRow(row) {
					continue
				}
				out = append(out, Finding{
					At:    at(fset, row.Pos()),
					Check: "keyed-rows",
					What:  "table row field is positional; name it",
				})
			}
			return true
		})
	}
	return out
}

// looksLikeRow reports whether a literal is a struct row rather than a nested
// slice or map.
//
// A row's type is either omitted, because the outer literal supplies it, or it
// names a struct. A literal whose type is a slice or a map is a nested
// collection and its elements are values rather than fields.
func looksLikeRow(row *ast.CompositeLit) bool {
	switch row.Type.(type) {
	case nil, *ast.Ident, *ast.SelectorExpr, *ast.StructType:
		return true
	default:
		return false
	}
}
