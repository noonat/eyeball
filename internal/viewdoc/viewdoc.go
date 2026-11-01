package viewdoc

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"

	"github.com/noonat/eyeball/internal/view"
)

// Scoped says an address is one of the project, tree and file pages, which the
// React app draws and whose view is read from /api.
func Scoped(address string) bool {
	path, _, _ := strings.Cut(address, "?")
	return strings.HasPrefix(path, "/projects/") || strings.Contains(path, "/files/")
}

// Page draws a view as the template pages once did: the top bar, banners, rows
// and tables with their old class names. A test about one of these screens
// reads its content by those names, and the screen is a React component now, so
// this stands in for the markup. It draws from the fields of the view and
// nothing else, so a view that lacks something a test reads fails the test.
//
// status is what the view answered. A 404 draws the not found page with its
// banner, and a body that is not a view's is the page the mux wrote.
func Page(status int, body []byte) (string, error) {
	var b strings.Builder
	if status == http.StatusNotFound {
		var missing view.NotFound
		if json.Unmarshal(body, &missing) != nil {
			return string(body), nil
		}
		b.WriteString(docBar("Not found", "/", "Queue", missing.WaitingCount))
		b.WriteString(`<div class="scroll"><div class="page">`)
		if missing.Notice != nil {
			b.WriteString(docNotice(*missing.Notice))
		}
		b.WriteString(`<div class="empty"><p class="headline">Nothing here</p><p>There is no page at this address.</p></div></div></div>`)
		return wrapDoc(b.String()), nil
	}
	var v struct {
		view.Project
		Kind view.FilesKind `json:"kind"`
		Tree *view.Tree     `json:"tree"`
		File *view.File     `json:"file"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return "", errors.Wrap(err, "read a view")
	}
	b.WriteString(docBar(v.Title, v.Back, v.BackLabel, v.WaitingCount))
	switch {
	case v.Tree != nil:
		b.WriteString(docTree(*v.Tree))
	case v.File != nil:
		b.WriteString(docFile(*v.File))
	default:
		b.WriteString(docProject(v.Project))
	}
	return wrapDoc(b.String()), nil
}

func wrapDoc(inner string) string {
	return `<!doctype html><html><body><div class="app">` + inner + `</div></body></html>`
}

func esc(s string) string {
	return html.EscapeString(s)
}

func docIcon(name string) string {
	return `<span class="i i-` + name + `" aria-hidden="true"></span>`
}

func docBar(title, back, label string, count *int32) string {
	var b strings.Builder
	b.WriteString(`<div class="head"><div class="topbar"><a class="back" href="` + esc(back) + `">` + docIcon("chevron-left") + esc(label) + `</a><h1>` + esc(title) + `</h1>`)
	switch {
	case count == nil:
	case *count == 0:
		b.WriteString(`<span class="eye shut" role="img" aria-label="Nothing waiting"></span>`)
	default:
		fmt.Fprintf(&b, `<span class="eye" role="img" aria-label="%d waiting"><span class="iris">%d</span></span>`, *count, *count)
	}
	b.WriteString(`</div></div>`)
	return b.String()
}

func docNotice(n view.Notice) string {
	class, glyph := "notice", "info"
	if n.Warn {
		class, glyph = "notice warn", "triangle-alert"
	}
	heading := ""
	if n.Heading != "" {
		heading = "<strong>" + esc(n.Heading) + "</strong> "
	}
	return `<div class="` + class + `"><span class="glyph i i-` + glyph + `" aria-hidden="true"></span><span>` + heading + esc(n.Text) + `</span></div>`
}

func docNotices(notices []view.Notice) string {
	var b strings.Builder
	for _, n := range notices {
		b.WriteString(docNotice(n))
	}
	return b.String()
}

func docCrumb(icon, dir, name string) string {
	lead := ""
	if dir != "" {
		lead = `<span class="dir"><bdi>` + esc(dir) + `</bdi></span>`
	}
	return `<div class="crumb">` + docIcon(icon) + lead + `<span class="name">` + esc(name) + `</span></div>`
}

func docCounts(added, removed string) string {
	if added == "" && removed == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<span class="counts">`)
	if added != "" {
		b.WriteString(`<span class="plus">+` + esc(added) + `</span>`)
	}
	if added != "" && removed != "" {
		b.WriteString(" ")
	}
	if removed != "" {
		b.WriteString(`<span class="minus">&minus;` + esc(removed) + `</span>`)
	}
	b.WriteString(`</span>`)
	return b.String()
}

func docTree(t view.Tree) string {
	var b strings.Builder
	b.WriteString(`<div class="scroll"><div class="page">` + docCrumb("folder", t.Dir, t.Name) + docNotices(t.Notices))
	if len(t.Rows) == 0 {
		b.WriteString(`<div class="empty"><p class="headline">Nothing here</p><p>` + esc(t.Empty) + `</p></div>`)
	} else {
		b.WriteString(`<div class="tree">`)
		for _, row := range t.Rows {
			class := "treerow"
			if row.Indent {
				class += " indent"
			}
			if row.InScope {
				class += " inscope"
			}
			b.WriteString(`<a class="` + class + `" href="` + esc(row.Href) + `">`)
			if row.Dir {
				b.WriteString(docIcon("folder") + `<span class="nm">` + esc(row.Name) + `</span>` + docIcon("chevron-right"))
			} else {
				b.WriteString(docIcon("file-text") + `<span class="nm">` + esc(row.Name) + `</span>` + docCounts(row.Added, row.Removed))
			}
			b.WriteString(`</a>`)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div></div>`)
	return b.String()
}

func docQueueRow(row view.Row, waiting bool) string {
	var b strings.Builder
	kind := "working"
	if waiting {
		kind = "waiting"
	}
	b.WriteString(`<a class="row ` + kind + `" href="/reviews/` + strconv.FormatInt(row.ID, 10) + `"><span class="where">`)
	if row.Project != "" {
		b.WriteString(`<span class="bit">` + docIcon("folder") + `<span class="project">` + esc(row.Project) + `</span></span>`)
	}
	b.WriteString(`<span class="bit">` + docIcon("bot") + `<span class="agent">` + esc(row.Agent) + `</span></span></span><span class="age">` + esc(row.Age) + `</span>`)
	if waiting {
		b.WriteString(`<span class="title"><mark>` + esc(row.Title) + `</mark></span>`)
		if row.Gist != "" {
			b.WriteString(`<span class="gist">` + esc(row.Gist) + `</span>`)
		}
		fmt.Fprintf(&b, `<span class="meta"><span>Round %d</span><span>%s</span><span class="counts"><span class="plus">+%s</span> <span class="minus">&minus;%s</span></span></span>`, row.Round, esc(row.Files), esc(row.Added), esc(row.Removed))
	} else {
		fmt.Fprintf(&b, `<span class="title">%s</span><span class="meta"><span>Changes requested, round %d</span></span>`, esc(row.Title), row.Round)
	}
	b.WriteString(`</a>`)
	return b.String()
}

func docCopyRows(c view.WorkingCopy) string {
	var b strings.Builder
	if len(c.Rows) == 0 {
		if c.Empty != nil {
			b.WriteString(`<div class="empty"><span class="headline">` + esc(c.Empty.Headline) + `</span><p>` + esc(c.Empty.Text) + `</p></div>`)
		}
		return b.String()
	}
	if c.Capped != nil {
		b.WriteString(docNotice(*c.Capped))
	}
	b.WriteString(`<div class="tree">`)
	for _, row := range c.Rows {
		class := "treerow inscope"
		if row.Claimed {
			class = "treerow"
		}
		b.WriteString(`<a class="` + class + `" href="` + esc(row.Href) + `">` + docIcon("file-text") + `<span class="nm">` + esc(row.Path) + `</span>`)
		if row.Claimed {
			b.WriteString(`<span class="pill plain">In review</span>`)
		} else {
			b.WriteString(docCounts(row.Added, row.Removed))
		}
		b.WriteString(`</a>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

func docCount(count string) string {
	if count == "" {
		return ""
	}
	return ` <span class="count">` + esc(count) + `</span>`
}

func docProject(p view.Project) string {
	var b strings.Builder
	b.WriteString(`<div class="scroll"><div class="page"><div class="brief"><div class="where"><span class="agent">` + esc(p.Root) + `</span></div><h2 class="title">` + esc(p.Name) + `</h2></div>`)
	if !p.Named && len(p.Copies) > 0 {
		b.WriteString(docNotices(p.Copies[0].Notices))
	}
	if p.Open > 0 {
		fmt.Fprintf(&b, `<div class="section"><h2>Open reviews <span class="count">%d</span></h2><div class="rows">`, p.Open)
		for _, row := range p.Waiting {
			b.WriteString(docQueueRow(row, true))
		}
		for _, row := range p.Working {
			b.WriteString(docQueueRow(row, false))
		}
		b.WriteString(`</div></div>`)
	}
	switch {
	case p.Named:
		if p.Live {
			b.WriteString(`<div class="notice"><span class="glyph i i-info" aria-hidden="true"></span><span>Everything changed on disk right now, live, in each working copy. Read only: a verdict belongs to a review.</span></div>`)
		}
		for _, c := range p.Copies {
			b.WriteString(`<div class="section"><h2>` + esc(c.Name) + docCount(c.Count) + `</h2><div class="hint">` + esc(c.Hint) + `</div>` + docNotices(c.Notices) + docCopyRows(c) + `</div>`)
		}
	case len(p.Copies) > 0:
		c := p.Copies[0]
		b.WriteString(`<div class="section"><h2>Working copy` + docCount(c.Count) + `</h2>`)
		if len(c.Rows) > 0 {
			b.WriteString(`<div class="notice"><span class="glyph i i-info" aria-hidden="true"></span><span>Everything changed on disk right now, live. Read only: a verdict belongs to a review.</span></div>`)
		}
		b.WriteString(docCopyRows(c) + `</div>`)
	}
	b.WriteString(`</div></div>`)
	return b.String()
}

func docModeline(mode string) string {
	return `<div class="modeline">` + docIcon("triangle-alert") + `<span>` + esc(mode) + `</span></div>`
}

func docToolbar(t view.Toolbar) string {
	var b strings.Builder
	b.WriteString(`<div class="toolbar">`)
	switch {
	case t.Note != "":
		b.WriteString(`<span class="counts"><span class="hint">` + esc(t.Note) + `</span></span>` + "\n")
	case t.Counted != "":
		b.WriteString(`<span class="counts"><span class="plus">+` + esc(t.Added) + `</span> <span class="minus">&minus;` + esc(t.Removed) + `</span> <span class="hint">` + esc(t.Counted) + `</span></span>` + "\n")
	}
	segmented := func(first, firstHref, second, secondHref string, secondShown bool) {
		current := ` aria-current="true"`
		one, two := current, ""
		if secondShown {
			one, two = "", current
		}
		b.WriteString(`<span class="segmented"><a href="` + esc(firstHref) + `"` + one + `>` + esc(first) + `</a><a href="` + esc(secondHref) + `"` + two + `>` + esc(second) + `</a></span>` + "\n")
	}
	switch {
	case t.Marked != "":
		segmented("Marked", t.Marked, "Plain", t.Plain, t.ShowPlain)
	case t.Rendered != "":
		segmented(t.RenderedLabel, t.Rendered, "Source", t.Source, t.ShowSource)
	}
	b.WriteString(`</div>`)
	return b.String()
}

func docRendered(r view.Rendering) string {
	var b strings.Builder
	switch {
	case r.Facts != nil:
		f := r.Facts
		b.WriteString(`<div class="file"><div class="filefacts"><span class="kind">` + esc(f.Kind) + `</span>`)
		if f.Size != "" {
			b.WriteString(`<span class="size">` + esc(f.Size) + `</span>`)
		}
		if f.Hint != "" {
			b.WriteString(`<span class="hint">` + esc(f.Hint) + `</span>`)
		}
		if f.Load != "" {
			b.WriteString(`<a class="load" href="` + esc(f.Load) + `">Load anyway</a>`)
		}
		b.WriteString(`</div></div>`)
	case r.Frame != "":
		b.WriteString(`<div class="preview"><div class="cap">`)
		if r.Sandboxed {
			b.WriteString(docIcon("triangle-alert") + `<span>Sandboxed</span>`)
		} else {
			b.WriteString(`<span>` + esc(r.Kind) + `</span><span class="dim">` + esc(r.Size) + `</span>`)
		}
		b.WriteString(`<a class="open" href="` + esc(r.Frame) + `" target="_blank" rel="noopener">` + docIcon("maximize-2") + `Open</a></div>`)
		sandbox := ""
		if r.Sandboxed {
			sandbox = ` sandbox="allow-scripts"`
		}
		b.WriteString(`<iframe class="pageframe"` + sandbox + ` loading="lazy" title="` + esc(r.Title) + `" src="` + esc(r.Frame) + `"></iframe></div>`)
	default:
		b.WriteString(`<div class="beforeafter">`)
		for _, s := range r.Sides {
			class := "preview"
			if s.Tint != "" {
				class += " " + s.Tint
			}
			b.WriteString(`<div class="` + class + `"><div class="cap"><span>` + esc(s.Label) + `</span><span class="dim">` + esc(s.Caption) + `</span></div>`)
			switch s.Element {
			case view.MediaElementVideo:
				b.WriteString(`<video controls playsinline preload="metadata" src="` + esc(s.Src) + `"></video>`)
			case view.MediaElementAudio:
				b.WriteString(`<audio controls preload="metadata" src="` + esc(s.Src) + `"></audio>`)
			default:
				b.WriteString(`<img class="shot natural" loading="lazy" src="` + esc(s.Src) + `" alt="` + esc(s.Label) + `">`)
			}
			b.WriteString(`</div>`)
		}
		b.WriteString(`</div>`)
	}
	return b.String()
}

var roleClasses = map[view.TokenRole]string{
	view.TokenRoleKeyword:  "k",
	view.TokenRoleString:   "s",
	view.TokenRoleComment:  "c",
	view.TokenRoleNumber:   "n",
	view.TokenRoleFunction: "f",
}

func docCode(rows []view.CodeRow) string {
	var b strings.Builder
	b.WriteString(`<div class="code narrow"><table>`)
	for _, row := range rows {
		if row.Kind == view.CodeRowKindHunk {
			b.WriteString(`<tr class="hunkhead"><td colspan="3">` + esc(row.Header) + `</td></tr>`)
			continue
		}
		class, number := "", fmt.Sprintf(` data-line="%d"`, row.Number)
		switch row.Kind {
		case view.CodeRowKindAdd:
			class = ` class="add"`
		case view.CodeRowKindDel:
			class, number = ` class="del"`, fmt.Sprintf(` data-old="%d"`, row.Number)
		}
		fmt.Fprintf(&b, `<tr%s%s><td class="ln">%d</td><td class="sign">%s</td><td class="src">`, class, number, row.Number, esc(row.Sign))
		for _, tok := range row.Tokens {
			text := esc(tok.Text)
			if role := roleClasses[tok.Role]; role != "" {
				text = `<span class="` + role + `">` + text + `</span>`
			}
			if tok.Mark {
				text = `<span class="w">` + text + `</span>`
			}
			b.WriteString(text)
		}
		b.WriteString(`</td></tr>`)
	}
	b.WriteString(`</table></div>`)
	return b.String()
}

func docFile(f view.File) string {
	var b strings.Builder
	b.WriteString(`<div class="scroll"><div class="page">` + docCrumb("file-text", f.Dir, f.Name) + docNotices(f.Notices))
	if f.Toolbar != nil {
		b.WriteString(docToolbar(*f.Toolbar))
	}
	if f.Mode != "" && (f.Body == view.FileBodyRendered || f.Body == view.FileBodyDocument) {
		b.WriteString(`<div class="file">` + docModeline(f.Mode) + `</div>`)
	}
	switch f.Body {
	case view.FileBodyRendered:
		b.WriteString(docRendered(*f.Rendered))
	case view.FileBodyDeleted:
		b.WriteString(`<div class="empty"><span class="headline">Deleted</span><p>` + esc(f.Deleted) + `</p></div>`)
	case view.FileBodyDocument:
		b.WriteString(`<div class="prose">` + docPassages(f.Passages) + `</div>`)
	default:
		b.WriteString(`<div class="file">`)
		if f.Mode != "" {
			b.WriteString(docModeline(f.Mode))
		}
		switch f.Body {
		case view.FileBodyBinary:
			b.WriteString(`<div class="toobig">Binary file, not shown as a diff.</div>`)
		case view.FileBodyTooLarge:
			b.WriteString(`<div class="toobig">Too large to show as a diff.</div>`)
		default:
			b.WriteString(docCode(f.Rows))
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div></div>`)
	return b.String()
}

// docPassages joins a document's parts as the template page wrote them, with a
// fold as a details element holding its run.
func docPassages(parts []view.DocPart) string {
	var b strings.Builder
	for _, part := range parts {
		if part.Passage != nil {
			b.WriteString(string(part.Passage.HTML))
			continue
		}
		fmt.Fprintf(&b, "<details class=\"fold\"><summary>%d unchanged passages</summary>\n", len(part.Fold))
		for _, p := range part.Fold {
			b.WriteString(string(p.HTML))
		}
		b.WriteString("</details>\n")
	}
	return b.String()
}
