package convention

import (
	"fmt"
	"go/ast"
	"go/token"
	"path/filepath"
	"strings"
)

// Finding is one violation: where it is and what is wrong with it.
type Finding struct {
	// At is the position, as file:line, relative to the repository root.
	At string
	// Check is the rule's name, matching its entry in docs/conventions.md.
	Check string
	// What says what is wrong, in the words a person would use.
	What string
}

// String renders a finding the way a compiler renders a diagnostic.
func (f Finding) String() string {
	return fmt.Sprintf("%s: %s (%s)", f.At, f.What, f.Check)
}

// Check is one rule, named for its entry in docs/conventions.md.
//
// Run receives every file of one package together, because some rules need more
// than one file to decide: a test's name resolves against the declarations of
// the package it tests, which is rarely the file the test is in.
type Check struct {
	// Name is the rule's name, and also the directory its fixture lives in.
	Name string
	// Run reports every violation it finds in one package.
	Run func(fset *token.FileSet, files []*ast.File) []Finding
}

// Checks is every rule this package enforces over Go source, in the order
// docs/conventions.md lists them.
//
// A check missing from here is a rule nothing enforces. A check here with no
// fixture under testdata is a check nobody has watched fail, which
// Test_eachCheckFlagsItsFixture refuses.
var Checks = []Check{
	{Name: "doc-comments", Run: docComments},
	{Name: "brace-lines", Run: braceLines},
	{Name: "argument-wrapping", Run: argumentWrapping},
}

// at renders a position as file:line, with the path relative to the repository
// root so a finding reads the same wherever the test was run from.
func at(fset *token.FileSet, pos token.Pos) string {
	p := fset.Position(pos)
	name := p.Filename
	if root, err := repoRoot(); err == nil {
		if rel, err := filepath.Rel(root, name); err == nil {
			name = rel
		}
	}
	return fmt.Sprintf("%s:%d", name, p.Line)
}

// docComments reports an exported name with no doc comment, or one whose
// comment does not start with a name it declares.
//
// One declaration can bind several names, and a comment starting with any of
// them satisfies the rule, since starting with all of them is impossible.
func docComments(fset *token.FileSet, files []*ast.File) []Finding {
	var out []Finding
	report := func(pos token.Pos, kind string, names []string, doc *ast.CommentGroup) {
		var exported []string
		for _, n := range names {
			if ast.IsExported(n) {
				exported = append(exported, n)
			}
		}
		if len(exported) == 0 {
			return
		}
		label := strings.Join(exported, ", ")
		if doc == nil || strings.TrimSpace(doc.Text()) == "" {
			out = append(out, Finding{
				At:    at(fset, pos),
				Check: "doc-comments",
				What:  fmt.Sprintf("exported %s %s has no doc comment", kind, label),
			})
			return
		}
		text := strings.TrimSpace(doc.Text())
		for _, n := range exported {
			if strings.HasPrefix(text, n+" ") || strings.HasPrefix(text, n+"\n") {
				return
			}
		}
		out = append(out, Finding{
			At:    at(fset, pos),
			Check: "doc-comments",
			What: fmt.Sprintf(
				"doc comment on %s %s starts with none of its names", kind, label),
		})
	}

	for _, file := range files {
		test := strings.HasSuffix(fset.Position(file.Pos()).Filename, "_test.go")
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if test && isTestFunc(d.Name.Name) {
					continue
				}
				kind := "func"
				if d.Recv != nil {
					kind = "method"
				}
				report(d.Pos(), kind, []string{d.Name.Name}, d.Doc)
			case *ast.GenDecl:
				reportGen(d, report)
			}
		}
	}
	return out
}

// isTestFunc reports whether a name is one the testing toolchain calls.
//
// Those need no doc comment. Their name is the documentation: the naming rule in
// docs/conventions.md requires it to say what is under test, and a comment
// repeating that adds a second copy to keep in step.
func isTestFunc(name string) bool {
	prefixes := []string{"Test", "Benchmark", "Fuzz", "Example"}
	for _, p := range prefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// reportGen walks a const, var or type block and reports each declaration in it.
//
// A grouped block can carry one comment for the group and one per spec. The
// spec's own comment wins, and the group's stands in when a spec has none, which
// is how a block of related constants is documented once.
func reportGen(
	d *ast.GenDecl,
	report func(token.Pos, string, []string, *ast.CommentGroup),
) {
	for _, spec := range d.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			doc := s.Doc
			if doc == nil {
				doc = d.Doc
			}
			report(s.Pos(), "type", []string{s.Name.Name}, doc)
			reportFields(s, report)
		case *ast.ValueSpec:
			doc := s.Doc
			if doc == nil {
				doc = d.Doc
			}
			names := make([]string, 0, len(s.Names))
			for _, n := range s.Names {
				names = append(names, n.Name)
			}
			report(s.Pos(), d.Tok.String(), names, doc)
		}
	}
}

// reportFields reports the exported fields of an exported struct type.
//
// Two fields declared separately are two declarations and need a comment each,
// which is why this reports per field rather than per struct.
func reportFields(
	s *ast.TypeSpec,
	report func(token.Pos, string, []string, *ast.CommentGroup),
) {
	if !ast.IsExported(s.Name.Name) {
		return
	}
	st, ok := s.Type.(*ast.StructType)
	if !ok || st.Fields == nil {
		return
	}
	for _, f := range st.Fields.List {
		if len(f.Names) == 0 {
			continue
		}
		names := make([]string, 0, len(f.Names))
		for _, n := range f.Names {
			names = append(names, n.Name)
		}
		report(f.Pos(), "field", names, f.Doc)
	}
}
