package convention

import (
	"regexp"
	"strings"
)

// cssClass matches a class selector in a stylesheet.
var cssClass = regexp.MustCompile(`\.([A-Za-z_][\w-]*)`)

// cssMask matches one row of the generated icon table.
var cssMask = regexp.MustCompile(`\.i-([a-z0-9-]+) \{\s*--icon:`)

// requiredDecls are the declarations app.css cannot lose.
//
// app.css holds a generated block. A generator that deletes more than its own
// block leaves a page that still renders, unstyled, which reads as a layout bug
// rather than as a missing rule. These are the declarations with no fallback.
//
// Each is anchored at the start of a line, because a bare substring matches the
// wrong rule: ".ic .i {" contains ".i {".
var requiredDecls = []string{
	`--prose:`, `--mono:`, `--t-md:`, `--pad:`, `--gap:`, `--r-md:`,
	`--ink:`, `--bg:`, `--add:`, `--del:`,
	`\.i \{`, `\.app \{`, `\.page \{`, `\.row \{`, `\.decide \{`,
}

// cssDeclarations reports a required declaration app.css has lost.
func cssDeclarations(t Tearout) []Finding {
	var out []Finding
	for _, decl := range requiredDecls {
		if regexp.MustCompile(`(?m)^\s*` + decl).MatchString(t.CSS) {
			continue
		}
		out = append(out, Finding{
			At:    tearoutDir + "/app.css",
			Check: "css-declarations",
			What:  "no longer declares " + strings.ReplaceAll(decl, `\`, ""),
		})
	}
	return out
}

// iconsListed reports a disagreement between icons.txt and the mask table.
//
// A class with no mask resolves to nothing and leaves a gap, which reads as a
// spacing bug rather than as a missing icon.
func iconsListed(t Tearout) []Finding {
	listed := map[string]struct{}{}
	for _, name := range t.Icons {
		listed[name] = struct{}{}
	}
	mapped := map[string]struct{}{}
	for _, m := range cssMask.FindAllStringSubmatch(t.CSS, -1) {
		mapped[m[1]] = struct{}{}
	}
	var out []Finding
	for _, name := range sortedSet(listed) {
		if _, ok := mapped[name]; ok {
			continue
		}
		out = append(out, Finding{
			At:    tearoutDir + "/icons.txt",
			Check: "icons-listed",
			What:  "lists " + name + " but app.css has no mask for it",
		})
	}
	for _, name := range sortedSet(mapped) {
		if _, ok := listed[name]; ok {
			continue
		}
		out = append(out, Finding{
			At:    tearoutDir + "/app.css",
			Check: "icons-listed",
			What:  "maps " + name + " but icons.txt does not list it",
		})
	}
	return out
}
