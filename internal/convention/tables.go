package convention

import (
	"fmt"
	"go/ast"
	"go/token"
	"os"
	"strings"
	"unicode/utf8"
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
				What:  "range over an anonymous literal; assign it to a variable first",
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

// Past these, a table row in a test puts every field on its own line.
const (
	rowFields = 3
	rowRunes  = 80
)

// rowLines reports a table row in a test that keeps too much on one line.
//
// A row on one line passes with at most rowFields fields and at most rowRunes
// runes from brace to brace. The row's own text is measured rather than its
// line, because a line's length depends on how deep it is indented and how wide
// a tab is drawn, and neither is part of the row. A row on several lines puts
// every field on a line of its own, with both braces alone, so a row cannot pass
// by breaking once in the middle.
//
// Only _test.go files are examined. The rule is about test tables, and the few
// tables outside tests are compact grids of short fields.
func rowLines(fset *token.FileSet, files []*ast.File) []Finding {
	var out []Finding
	for _, file := range files {
		name := fset.Position(file.Pos()).Filename
		if !strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			out = append(out, Finding{At: at(fset, file.Pos()), Check: "row-lines", What: "cannot read the file to measure its rows: " + err.Error()})
			continue
		}
		ast.Inspect(file, func(n ast.Node) bool {
			outer, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			for _, elt := range outer.Elts {
				row, ok := elt.(*ast.CompositeLit)
				if !ok || len(row.Elts) == 0 || !looksLikeRow(row) {
					continue
				}
				out = append(out, rowFindings(fset, src, row)...)
			}
			return true
		})
	}
	return out
}

// rowFindings reports what is wrong with one row's layout, if anything.
func rowFindings(fset *token.FileSet, src []byte, row *ast.CompositeLit) []Finding {
	line := func(p token.Pos) int { return fset.Position(p).Line }
	finding := func(pos token.Pos, what string) Finding {
		return Finding{At: at(fset, pos), Check: "row-lines", What: what}
	}

	if line(row.Lbrace) == line(row.Rbrace) {
		if len(row.Elts) > rowFields {
			return []Finding{finding(row.Lbrace, fmt.Sprintf("table row holds %d fields on one line; put each on its own line", len(row.Elts)))}
		}
		if width := rowWidth(fset, src, row); width > rowRunes {
			return []Finding{finding(row.Lbrace, fmt.Sprintf("table row is %d runes wide on one line; put each field on its own line", width))}
		}
		return nil
	}

	var out []Finding
	prev := line(row.Lbrace)
	for _, elt := range row.Elts {
		if line(elt.Pos()) == prev {
			out = append(out, finding(elt.Pos(), "table row field shares a line; put each field on its own line"))
		}
		prev = line(elt.End())
	}
	if line(row.Rbrace) == prev {
		out = append(out, finding(row.Rbrace, "table row's closing brace shares a line with its last field"))
	}
	return out
}

// rowWidth is the number of runes from a one-line row's opening brace to its
// closing brace, inclusive. Columns count bytes, so a row holding multibyte text
// is measured from the source.
func rowWidth(fset *token.FileSet, src []byte, row *ast.CompositeLit) int {
	start, end := fset.Position(row.Lbrace).Offset, fset.Position(row.Rbrace).Offset
	return utf8.RuneCount(src[start : end+1])
}
