package convention

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cockroachdb/errors"
)

// ProseCheck is one rule over a committed markdown file, named for its entry in
// docs/conventions.md.
//
// Run takes a whole file rather than a parsed tree. The prose rules are about
// wording, and markdown has no structure worth building for them beyond knowing
// which lines are prose at all.
type ProseCheck struct {
	// Name is the rule's name, and also the directory its fixture lives in.
	Name string
	// Run reports every violation it finds in one file.
	Run func(path string, body string) []Finding
}

// ProseChecks is every rule this package enforces over markdown, in the order
// docs/conventions.md lists them.
var ProseChecks = []ProseCheck{
	{Name: "prose-person", Run: prosePerson},
	{Name: "prose-dashes", Run: proseDashes},
}

// personWords are the first and second person pronouns docs/voice.md bans.
var personWords = []string{
	"I", "I'm", "I've", "I'd", "I'll", "me", "my", "mine", "myself",
	"we", "we're", "we've", "we'd", "we'll", "us", "our", "ours", "ourselves",
	"you", "you're", "you've", "you'd", "you'll", "your", "yours", "yourself",
}

// personPattern matches any of personWords as a whole word, in any case.
var personPattern = regexp.MustCompile(`(?i)\b(?:` + strings.Join(personWords, "|") + `)\b`)

// inlineCode matches a code span, which names something rather than says it.
var inlineCode = regexp.MustCompile("`[^`\n]*`")

// quotedSpan matches a double-quoted example on one line.
var quotedSpan = regexp.MustCompile(`"[^"\n]*"`)

// prosePerson reports a first or second person pronoun in prose.
//
// This is the voice rule broken most often by accident, because the person
// writing is the one the rule is about and the pronoun arrives without being
// chosen.
func prosePerson(path string, body string) []Finding {
	var out []Finding
	for i, line := range proseLines(body) {
		for _, m := range personPattern.FindAllString(line, -1) {
			out = append(out, Finding{
				At:    fmt.Sprintf("%s:%d", path, i+1),
				Check: "prose-person",
				What:  "first or second person: " + m,
			})
		}
	}
	return out
}

// proseDashes reports an em dash in prose.
//
// docs/voice.md says a period beats a dash. The character is worth its own rule
// because it arrives without being typed, from a keyboard substitution or from
// text written elsewhere and pasted in.
func proseDashes(path string, body string) []Finding {
	var out []Finding
	for i, line := range proseLines(body) {
		if !strings.Contains(line, "—") {
			continue
		}
		out = append(out, Finding{
			At:    fmt.Sprintf("%s:%d", path, i+1),
			Check: "prose-dashes",
			What:  "em dash in prose; a comma, a colon or a period says it",
		})
	}
	return out
}

// proseLines returns one entry per line of a markdown file, with everything
// that is not this repository's own prose blanked out. Blanking rather than
// dropping keeps the index equal to the line number.
//
// A fenced block is code. A blockquote is quoted material, which covers the
// annotations backlog writes into a spec when it closes an iteration. An inline
// code span names something rather than says it, and a double-quoted span is an
// example, which is how docs/voice.md states the rule against the first person
// without breaking it.
func proseLines(body string) []string {
	lines := strings.Split(body, "\n")
	out := make([]string, len(lines))
	fenced := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			fenced = !fenced
			continue
		}
		if fenced || strings.HasPrefix(trimmed, ">") {
			continue
		}
		out[i] = quotedSpan.ReplaceAllString(inlineCode.ReplaceAllString(line, " "), " ")
	}
	return out
}

// MarkdownFiles lists every committed markdown file, relative to the root.
//
// The list comes from git for the same reason the Makefile's does: a new file
// is untracked until it is added, and a check that reads only the index passes
// while checking nothing.
//
// A symlink is skipped. CLAUDE.md points at AGENTS.md, and reading both reports
// every finding twice. testdata is skipped because its fixtures break the rules
// on purpose.
func MarkdownFiles() ([]string, error) {
	root, err := repoRoot()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command("git", "ls-files", "--cached", "--others", "--exclude-standard", "*.md")
	cmd.Dir = root
	body, err := cmd.Output()
	if err != nil {
		return nil, errors.Wrap(err, "git ls-files")
	}
	var out []string
	for _, name := range strings.Split(strings.TrimSpace(string(body)), "\n") {
		if name == "" || strings.Contains(name, "/testdata/") {
			continue
		}
		info, err := os.Lstat(filepath.Join(root, name))
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		out = append(out, name)
	}
	return out, nil
}

// ReadMarkdown reads one file named relative to the repository root.
func ReadMarkdown(name string) (string, error) {
	root, err := repoRoot()
	if err != nil {
		return "", err
	}
	body, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		return "", errors.Wrapf(err, "read %s", name)
	}
	return string(body), nil
}

// ProseFixture reads one check's markdown fixture under testdata.
func ProseFixture(check string) (string, string, error) {
	name := filepath.Join("internal", "convention", "testdata", check, "bad.md")
	body, err := ReadMarkdown(name)
	if err != nil {
		return "", "", err
	}
	return filepath.ToSlash(name), body, nil
}
