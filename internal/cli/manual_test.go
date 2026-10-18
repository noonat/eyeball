package cli

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/noonat/eyeball/internal/api"
	. "github.com/onsi/gomega"
	ucli "github.com/urfave/cli/v3"
)

// manualSections maps each heading under the manual's JSON fields to the type
// that command prints with --json.
var manualSections = map[string]any{
	"request":       api.RequestResult{},
	"wait":          api.WaitResult{},
	"status":        api.StatusResult{},
	"show":          api.ShowResult{},
	"reply":         api.ReplyResult{},
	"abandon":       api.AbandonResult{},
	"check":         api.CheckResult{},
	"cat":           api.CatResult{},
	"start":         api.Daemon{},
	"stop":          api.StopResult{},
	"tailnet login": api.LoginResult{},
	"refusal":       api.ErrorBody{},
}

func Test_manualChecksCanFail(t *testing.T) {
	// Each check is proved against the manual with one entry removed, so a
	// check that found nothing because it looked at nothing would fail here.
	g := NewWithT(t)
	g.Expect(missingCommandsAndFlags(strings.ReplaceAll(manual, "`eyeball abandon`", ""))).To(ContainElement("eyeball abandon"))
	g.Expect(missingCommandsAndFlags(strings.ReplaceAll(manual, "`--timeout`", ""))).To(ContainElement("--timeout"))
	g.Expect(missingCommandsAndFlags(strings.ReplaceAll(manual, "`eyeball tailnet login`", ""))).To(ContainElement("eyeball tailnet login"))
	g.Expect(missingFields(strings.ReplaceAll(manual, "`comments.late`", ""))).To(ContainElement("wait: comments.late"))
}

func Test_manualNamesEveryCodeAndVariable(t *testing.T) {
	g := NewWithT(t)
	codes := refusalCodes(g)
	g.Expect(codes).To(ContainElement("path_claimed"), "the parse found the codes")
	variables := []string{"EYEBALL_AGENT", "EYEBALL_NO_START", "XDG_STATE_HOME"}
	for _, name := range append(codes, variables...) {
		g.Expect(manual).To(ContainSubstring("`"+name+"`"), name)
	}
}

func Test_manualNamesEveryCommandAndFlag(t *testing.T) {
	g := NewWithT(t)
	g.Expect(missingCommandsAndFlags(manual)).To(BeEmpty())
}

func Test_manualNamesEveryField(t *testing.T) {
	g := NewWithT(t)
	g.Expect(missingFields(manual)).To(BeEmpty())
}

// missingCommandsAndFlags lists every command, as `eyeball <name>` or
// `eyeball <group> <name>`, and every flag, as `--<name>`, that the manual
// does not show in a code span.
func missingCommandsAndFlags(text string) []string {
	var missing []string
	root := (&app{env: Env{Getenv: func(string) string { return "" }}}).command()
	check := func(span string) {
		if !strings.Contains(text, "`"+span+"`") {
			missing = append(missing, span)
		}
	}
	flags := func(cmd *ucli.Command) {
		for _, f := range cmd.Flags {
			for _, name := range f.Names() {
				if len(name) == 1 {
					check("-" + name)
				} else {
					check("--" + name)
				}
			}
		}
	}
	flags(root)
	var walk func(prefix string, cmds []*ucli.Command)
	walk = func(prefix string, cmds []*ucli.Command) {
		for _, cmd := range cmds {
			// A group such as tailnet does nothing itself. Its commands are
			// what the manual names.
			if len(cmd.Commands) == 0 {
				check(prefix + cmd.Name)
			}
			flags(cmd)
			walk(prefix+cmd.Name+" ", cmd.Commands)
		}
	}
	walk("eyeball ", root.Commands)
	return missing
}

// missingFields lists every JSON field, as section: dotted.path, that the
// manual's section for its command does not show in a code span.
func missingFields(text string) []string {
	_, reference, found := strings.Cut(text, "## JSON fields")
	if !found {
		return []string{"the manual has no JSON fields section"}
	}
	var missing []string
	for section, value := range manualSections {
		_, body, found := strings.Cut(reference, "\n### "+section+"\n")
		if !found {
			missing = append(missing, section+": the section")
			continue
		}
		if next := strings.Index(body, "\n### "); next >= 0 {
			body = body[:next]
		}
		for _, path := range jsonPaths(reflect.TypeOf(value), "") {
			if !strings.Contains(body, "`"+path+"`") {
				missing = append(missing, section+": "+path)
			}
		}
	}
	return missing
}

// jsonPaths lists the dotted JSON paths of every field a type encodes, walking
// into structs and through slices and pointers to them.
func jsonPaths(t reflect.Type, prefix string) []string {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == reflect.TypeOf(time.Time{}) {
		return []string{prefix}
	}
	switch t.Kind() {
	case reflect.Struct:
		var out []string
		for i := range t.NumField() {
			f := t.Field(i)
			name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if f.Anonymous && name == "" {
				out = append(out, jsonPaths(f.Type, prefix)...)
				continue
			}
			path := name
			if prefix != "" {
				path = prefix + "." + name
			}
			out = append(out, jsonPaths(f.Type, path)...)
		}
		return out
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Struct {
			return jsonPaths(t.Elem(), prefix)
		}
	}
	return []string{prefix}
}

// refusalCodes reads every api.Code constant's value from the package source,
// since Go cannot enumerate a package's constants by reflection.
func refusalCodes(g *WithT) []string {
	g.THelper()
	file, err := parser.ParseFile(token.NewFileSet(), "../api/api.go", nil, 0)
	g.Expect(err).NotTo(HaveOccurred())
	var codes []string
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs := spec.(*ast.ValueSpec)
			ident, ok := vs.Type.(*ast.Ident)
			if !ok || ident.Name != "Code" || len(vs.Values) != 1 {
				continue
			}
			lit, ok := vs.Values[0].(*ast.BasicLit)
			if !ok {
				continue
			}
			value, err := strconv.Unquote(lit.Value)
			g.Expect(err).NotTo(HaveOccurred())
			codes = append(codes, value)
		}
	}
	return codes
}
