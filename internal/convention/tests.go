package convention

import (
	"go/ast"
	"go/token"
	"strings"
	"unicode"
)

// subject is a test name taken apart: the identifier it tests, the method, and
// the description of a narrow case. Any of the three may be empty.
type subject struct {
	// X is the package-level type or function under test.
	X string
	// Y is the method of X under test.
	Y string
	// Z is the description of one narrow case.
	Z string
}

// decls is what a package declares at the top level, for resolving a test name.
type decls struct {
	// names are the package-level types and functions.
	names map[string]struct{}
	// methods are the method names of each type.
	methods map[string]map[string]struct{}
}

// testNames reports a test whose name does not say what it tests.
//
// The shapes are the ones go vet already enforces for examples: TestX,
// TestX_Y, and a third segment that starts lowercase for one narrow case. Vet
// applies none of this to tests, which is why it is here.
func testNames(fset *token.FileSet, files []*ast.File) []Finding {
	d := declared(files)
	var out []Finding
	for _, file := range files {
		if !isTestFile(fset, file) {
			continue
		}
		for _, fn := range testFuncs(file) {
			s, err := parseSubject(fn.Name.Name)
			if err != "" {
				out = append(out, Finding{At: at(fset, fn.Pos()), Check: "test-names", What: err})
				continue
			}
			if what := resolve(s, d); what != "" {
				out = append(out, Finding{At: at(fset, fn.Pos()), Check: "test-names", What: what})
			}
		}
	}
	return out
}

// resolve reports why a subject does not name something the package declares.
func resolve(s subject, d decls) string {
	if s.X == "" {
		return ""
	}
	if _, ok := d.names[s.X]; !ok {
		return "test names " + s.X + ", which the package does not declare"
	}
	if s.Y == "" {
		return ""
	}
	if _, ok := d.methods[s.X][s.Y]; !ok {
		return "test names " + s.X + "." + s.Y + ", which is not a method of " + s.X
	}
	return ""
}

// parseSubject takes a test's name apart, or says what is wrong with it.
//
// Test_description is the package-level form, for a test whose subject is the
// package rather than an identifier. An underscore is not a lowercase letter, so
// the toolchain still runs it.
func parseSubject(name string) (subject, string) {
	rest := strings.TrimPrefix(name, "Test")
	if rest == "" {
		return subject{}, ""
	}
	parts := strings.Split(rest, "_")
	if parts[0] == "" {
		if len(parts) != 2 || !startsLower(parts[1]) {
			return subject{}, "package-level test should be Test_description, with the description starting lowercase"
		}
		return subject{Z: parts[1]}, ""
	}
	switch len(parts) {
	case 1:
		return subject{X: parts[0]}, ""
	case 2:
		if startsLower(parts[1]) {
			return subject{X: parts[0], Z: parts[1]}, ""
		}
		return subject{X: parts[0], Y: parts[1]}, ""
	case 3:
		if startsLower(parts[1]) {
			return subject{}, "a method name must start uppercase: " + parts[1]
		}
		if !startsLower(parts[2]) {
			return subject{}, "a description must start lowercase: " + parts[2]
		}
		return subject{X: parts[0], Y: parts[1], Z: parts[2]}, ""
	default:
		return subject{}, "test name has more than three segments"
	}
}

// startsLower reports whether a segment begins with a lowercase letter.
func startsLower(s string) bool {
	if s == "" {
		return false
	}
	return unicode.IsLower([]rune(s)[0])
}

// testOrder reports tests for one subject that are out of order.
//
// The order is the tuple X, Y, Z with an empty segment first, which is not the
// same as sorting the names as strings: an underscore sorts after the uppercase
// letters, so plain alphabetical puts TestStore_Freeze above TestStore_emptyBase
// and buries the type-level case under the methods.
func testOrder(fset *token.FileSet, files []*ast.File) []Finding {
	var out []Finding
	for _, file := range files {
		if !isTestFile(fset, file) {
			continue
		}
		var prev subject
		var prevName string
		for _, fn := range testFuncs(file) {
			s, err := parseSubject(fn.Name.Name)
			if err != "" {
				continue
			}
			if prevName != "" && less(s, prev) {
				out = append(out, Finding{
					At:    at(fset, fn.Pos()),
					Check: "test-order",
					What:  fn.Name.Name + " sorts before " + prevName + ", which is above it",
				})
			}
			prev, prevName = s, fn.Name.Name
		}
	}
	return out
}

// less orders two subjects on X, then Y, then Z, with an empty segment first.
func less(a, b subject) bool {
	if a.X != b.X {
		return a.X < b.X
	}
	if a.Y != b.Y {
		return a.Y < b.Y
	}
	return a.Z < b.Z
}

// isTestFile reports whether a file's name ends in _test.go.
func isTestFile(fset *token.FileSet, file *ast.File) bool {
	return strings.HasSuffix(fset.Position(file.Pos()).Filename, "_test.go")
}

// testFuncs lists the test functions in a file, in the order they are written.
func testFuncs(file *ast.File) []*ast.FuncDecl {
	var out []*ast.FuncDecl
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "Test") {
			continue
		}
		out = append(out, fn)
	}
	return out
}

// declared collects the package-level types, functions and methods that a test
// name can refer to.
func declared(files []*ast.File) decls {
	d := decls{
		names:   map[string]struct{}{},
		methods: map[string]map[string]struct{}{},
	}
	for _, file := range files {
		for _, decl := range file.Decls {
			switch t := decl.(type) {
			case *ast.FuncDecl:
				if t.Recv == nil {
					d.names[t.Name.Name] = struct{}{}
					continue
				}
				owner := receiverType(t.Recv)
				if owner == "" {
					continue
				}
				if d.methods[owner] == nil {
					d.methods[owner] = map[string]struct{}{}
				}
				d.methods[owner][t.Name.Name] = struct{}{}
			case *ast.GenDecl:
				for _, spec := range t.Specs {
					if ts, ok := spec.(*ast.TypeSpec); ok {
						d.names[ts.Name.Name] = struct{}{}
					}
				}
			}
		}
	}
	return d
}

// receiverType is the type name a method is declared on, without its pointer.
func receiverType(recv *ast.FieldList) string {
	if len(recv.List) == 0 {
		return ""
	}
	expr := recv.List[0].Type
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	if id, ok := expr.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}
