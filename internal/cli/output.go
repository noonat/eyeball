package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/template"

	"github.com/cockroachdb/errors"
	"github.com/noonat/eyeball/internal/api"
)

// output is how a command prints what the daemon answered.
type output struct {
	json   bool
	format string
	stdout io.Writer
	stderr io.Writer
}

// emit prints a successful answer: the body as it arrived under --json, the
// template's output under --format, and the human form otherwise.
func (o output) emit(raw []byte, human func(w io.Writer)) error {
	switch {
	case o.json:
		_, err := o.stdout.Write(withNewline(raw))
		return errors.WithStack(err)
	case o.format != "":
		return o.render(raw)
	default:
		// A write that fails here must fail the command: wait posts its
		// receipt only when this returns nil, and a comment stamped delivered
		// that never reached the screen is a lost comment.
		w := &errWriter{w: o.stdout}
		human(w)
		return errors.Wrap(w.err, "print the answer")
	}
}

// errWriter keeps the first error a sequence of writes hit, so a printer that
// ignores each write's error can still be checked once at the end.
type errWriter struct {
	w   io.Writer
	err error
}

// Write writes unless an earlier write failed.
func (e *errWriter) Write(p []byte) (int, error) {
	if e.err != nil {
		return 0, e.err
	}
	n, err := e.w.Write(p)
	e.err = err
	return n, err
}

// render runs the --format template over the body decoded as JSON, so a
// template addresses the names --json shows. Numbers become int64 where they
// are whole and float64 otherwise: decoded as float64 a size of twenty million
// prints as 2e+07, and left as json.Number they cannot be compared with gt or
// eq.
func (o output) render(raw []byte) error {
	tmpl, err := template.New("format").Option("missingkey=error").Parse(o.format)
	if err != nil {
		return &usageError{message: "the --format template does not parse: " + err.Error(), fix: "check the template's syntax; the field names are the ones --json prints"}
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return errors.Wrap(err, "decode the answer")
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, numbers(value)); err != nil {
		return &usageError{message: "the --format template failed: " + err.Error(), fix: "check the field names against what --json prints"}
	}
	_, err = o.stdout.Write(withNewline(buf.Bytes()))
	return errors.WithStack(err)
}

// numbers replaces every json.Number in a decoded value with an int64 or a
// float64.
func numbers(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			t[k] = numbers(child)
		}
		return t
	case []any:
		for i, child := range t {
			t[i] = numbers(child)
		}
		return t
	case json.Number:
		if n, err := t.Int64(); err == nil {
			return n
		}
		if f, err := t.Float64(); err == nil {
			return f
		}
		return t.String()
	default:
		return v
	}
}

// refuse prints a refusal the daemon sent: its body on stdout under --json,
// because an agent that asked for JSON parses stdout, and its message and fix
// on stderr otherwise.
func (o output) refuse(raw []byte) {
	if o.json {
		_, _ = o.stdout.Write(withNewline(raw))
		return
	}
	var body api.ErrorBody
	if err := json.Unmarshal(raw, &body); err != nil || body.Error.Message == "" {
		_, _ = fmt.Fprintf(o.stderr, "eyeball: the daemon refused with a body that is not a refusal: %s\n", strings.TrimSpace(string(raw)))
		return
	}
	o.printRefusal(body.Error)
}

// printRefusal prints one refusal in whichever form was asked for.
func (o output) printRefusal(e api.Error) {
	if o.json {
		raw, _ := json.MarshalIndent(api.ErrorBody{Error: e}, "", "  ")
		_, _ = o.stdout.Write(withNewline(raw))
		return
	}
	_, _ = fmt.Fprintf(o.stderr, "eyeball: %s\n", e.Message)
	if e.Fix != "" {
		_, _ = fmt.Fprintf(o.stderr, "fix: %s\n", e.Fix)
	}
}

// withNewline ends b with exactly one newline.
func withNewline(b []byte) []byte {
	return append(bytes.TrimRight(b, "\n"), '\n')
}

// plural writes a count with its noun.
func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// sizeText is a round's size the way the queue writes it.
func sizeText(s api.Size) string {
	return fmt.Sprintf("%s, +%d -%d", plural(s.Files, "file"), s.Added, s.Removed)
}

// printNext writes the next line, indenting any continuation so it reads as
// one instruction.
func printNext(w io.Writer, next string) {
	if next == "" {
		return
	}
	_, _ = fmt.Fprintf(w, "next: %s\n", strings.ReplaceAll(next, "\n", "\n      "))
}

// printComment writes one comment the way an agent reads it: where it points,
// what it quotes, and what it says.
func printComment(w io.Writer, c api.Comment) {
	where := "the whole review"
	switch {
	case c.StartLine > 0 && c.EndLine > c.StartLine:
		where = fmt.Sprintf("%s lines %d-%d", c.Path, c.StartLine, c.EndLine)
	case c.StartLine > 0:
		where = fmt.Sprintf("%s line %d", c.Path, c.StartLine)
	case c.Path != "":
		where = c.Path
	}
	late := ""
	if c.Late {
		late = ", written after the verdict"
	}
	_, _ = fmt.Fprintf(w, "\ncomment %d on %s (round %d%s)\n", c.ID, where, c.Round, late)
	for line := range strings.Lines(c.Quoted) {
		_, _ = fmt.Fprintf(w, "  > %s\n", strings.TrimRight(line, "\n"))
	}
	for line := range strings.Lines(c.Body) {
		_, _ = fmt.Fprintf(w, "  %s\n", strings.TrimRight(line, "\n"))
	}
	if c.Reply != "" {
		_, _ = fmt.Fprintf(w, "  reply: %s\n", c.Reply)
	}
}
