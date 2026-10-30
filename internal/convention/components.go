package convention

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cockroachdb/errors"
)

// componentDir is where the surface's React sources live, relative to the root.
const componentDir = "web/src"

// dynamicMark stands in for a template literal's `${...}`, whose value is only
// known when the component renders.
const dynamicMark = "\x00"

// readComponents reads every .ts and .tsx source and every .css file under a
// directory. Sources are returned as pages and stylesheets joined, with
// comments removed. A directory that does not exist holds nothing.
func readComponents(dir string, label string) ([]TearoutPage, string, error) {
	var pages []TearoutPage
	var css strings.Builder
	walk := func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		ext := filepath.Ext(path)
		if ext != ".ts" && ext != ".tsx" && ext != ".css" {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return errors.Wrapf(err, "read %s", path)
		}
		if ext == ".css" {
			css.WriteString(cssComment.ReplaceAllString(string(body), ""))
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return errors.WithStack(err)
		}
		name := filepath.ToSlash(filepath.Join(label, rel))
		pages = append(pages, TearoutPage{Name: name, Body: string(body)})
		return nil
	}
	if _, err := os.Stat(dir); err != nil {
		return nil, "", nil
	}
	if err := filepath.WalkDir(dir, walk); err != nil {
		return nil, "", errors.Wrapf(err, "walk %s", dir)
	}
	sort.Slice(pages, func(i, j int) bool {
		return pages[i].Name < pages[j].Name
	})
	return pages, css.String(), nil
}

// componentClasses reports a class a component names that no stylesheet
// defines.
//
// A component is held to app.css, which is what the surface ships. A story is
// held to every stylesheet, since it draws specimen chrome of its own. The
// class lists read are the ones a component writes as a literal: a className
// attribute and the arguments of cx. A name assembled at render time, such as
// the tone in `pill ${tone}`, is not read, so a union of tones is not checked.
//
// An undefined class still renders its text, unstyled, so the page looks close
// enough to right that nobody goes looking for the missing rule.
func componentClasses(t Tearout) []Finding {
	app := definedClasses(t.AppCSS)
	every := definedClasses(t.CSS + t.SrcCSS)
	var out []Finding
	for _, page := range t.Components {
		defined := app
		if strings.Contains(page.Name, ".stories.") || strings.Contains(page.Name, "/stories/") {
			defined = every
		}
		seen := map[string]struct{}{}
		for _, use := range classLiterals(page.Body) {
			for _, name := range strings.Fields(use.text) {
				if strings.Contains(name, dynamicMark) {
					continue
				}
				if _, ok := defined[name]; ok {
					continue
				}
				if _, ok := seen[name]; ok {
					continue
				}
				seen[name] = struct{}{}
				out = append(out, Finding{
					At:    tearoutAt(page, use.line),
					Check: "component-classes",
					What:  "class ." + name + " is used in a component but no stylesheet it may use defines it",
				})
			}
		}
	}
	return out
}

// definedClasses lists the class selectors in a stylesheet.
func definedClasses(css string) map[string]struct{} {
	defined := map[string]struct{}{}
	for _, m := range cssClass.FindAllStringSubmatch(css, -1) {
		defined[m[1]] = struct{}{}
	}
	return defined
}

// classLiteral is the text of one string a component writes as a class list.
type classLiteral struct {
	text string
	line int
}

// classLiterals finds the strings in a source that name classes: the value of
// every className attribute and the arguments of every cx call.
func classLiterals(src string) []classLiteral {
	var out []classLiteral
	for _, start := range classSites(src) {
		out = append(out, stringsIn(src, start)...)
	}
	return out
}

// classSites returns where each class list starts: after `className=` and after
// `cx(`. A site is the index of the first character to read, and reading
// continues through one string or one balanced group.
func classSites(src string) []int {
	var sites []int
	markers := []string{"className=", "cx("}
	for _, marker := range markers {
		from := 0
		for {
			at := strings.Index(src[from:], marker)
			if at < 0 {
				break
			}
			sites = append(sites, from+at+len(marker))
			from += at + len(marker)
		}
	}
	sort.Ints(sites)
	return sites
}

// stringsIn reads the strings of the expression starting at one index. A quote
// reads one string. An opening brace or parenthesis reads every string up to
// its match, template literals included.
func stringsIn(src string, start int) []classLiteral {
	if start >= len(src) {
		return nil
	}
	var end int
	switch src[start] {
	case '"', '\'', '`':
		end = skipString(src, start)
	case '{', '(':
		end = skipGroup(src, start)
	default:
		return nil
	}
	var out []classLiteral
	for i := start; i < end; {
		if c := src[i]; c == '"' || c == '\'' || c == '`' {
			next := skipString(src, i)
			out = append(out, classLiteral{
				text: stringBody(src[i+1 : next-1]),
				line: 1 + strings.Count(src[:i], "\n"),
			})
			i = next
			continue
		}
		i++
	}
	return out
}

// stringBody removes a template literal's substitutions from a string's text.
func stringBody(body string) string {
	var b strings.Builder
	for i := 0; i < len(body); i++ {
		if body[i] == '$' && i+1 < len(body) && body[i+1] == '{' {
			i = skipGroup(body, i+1) - 1
			b.WriteString(dynamicMark)
			continue
		}
		b.WriteByte(body[i])
	}
	return b.String()
}

// skipString returns the index just past the string starting at i.
func skipString(src string, i int) int {
	quote := src[i]
	for j := i + 1; j < len(src); j++ {
		switch {
		case src[j] == '\\':
			j++
		case quote == '`' && src[j] == '$' && j+1 < len(src) && src[j+1] == '{':
			j = skipGroup(src, j+1) - 1
		case src[j] == quote:
			return j + 1
		}
	}
	return len(src)
}

// skipGroup returns the index just past the brace or parenthesis group starting
// at i, stepping over any string inside it.
func skipGroup(src string, i int) int {
	depth := 0
	for j := i; j < len(src); j++ {
		switch src[j] {
		case '"', '\'', '`':
			j = skipString(src, j) - 1
		case '{', '(':
			depth++
		case '}', ')':
			depth--
			if depth == 0 {
				return j + 1
			}
		}
	}
	return len(src)
}
