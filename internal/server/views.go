package server

import (
	"html/template"

	"github.com/noonat/eyeball/internal/render"
	"github.com/noonat/eyeball/internal/view"
)

// projectView is the project's page as the JSON view holds it.
func projectView(p page, body projectPage, waiting *int32) view.Project {
	out := view.Project{
		Title:        p.Title,
		Back:         p.Back,
		BackLabel:    p.BackLabel,
		WaitingCount: waiting,
		Name:         body.Name,
		Root:         body.Root,
		Open:         int32(body.Open),
		Waiting:      body.Waiting,
		Working:      body.Working,
		Live:         body.Live,
		Named:        body.Named,
	}
	for _, c := range body.Copies {
		section := view.WorkingCopy{
			Name:    c.Name,
			Hint:    c.Hint,
			Notices: viewNotices(c.Notices),
			Count:   c.Count,
			Capped:  viewNoticePtr(c.Capped),
		}
		for _, row := range c.Rows {
			section.Rows = append(section.Rows, view.WorkingRow(row))
		}
		if c.Empty != nil {
			box := view.EmptyBox(*c.Empty)
			section.Empty = &box
		}
		out.Copies = append(out.Copies, section)
	}
	return out
}

// treeFilesView is a directory's page as the JSON view holds it.
func treeFilesView(p page, body treePage, waiting *int32) view.Files {
	tree := view.Tree{
		Dir:     body.Dir,
		Name:    body.Name,
		Notices: viewNotices(body.Notices),
		Empty:   body.Empty,
	}
	for _, row := range body.Rows {
		tree.Rows = append(tree.Rows, view.TreeRow(row))
	}
	out := filesView(p, waiting)
	out.Kind, out.Tree = view.FilesKindTree, &tree
	return out
}

// fileFilesView is a file's page as the JSON view holds it.
func fileFilesView(p page, body filePage, waiting *int32) view.Files {
	file := view.File{
		Dir:     body.Dir,
		Name:    body.Name,
		Notices: viewNotices(body.Notices),
		Mode:    body.Mode,
		Body:    fileBody(body),
	}
	if t := body.Toolbar; t != nil {
		toolbar := view.Toolbar(*t)
		file.Toolbar = &toolbar
	}
	switch file.Body {
	case view.FileBodyRendered:
		file.Rendered = viewRendering(body.Rendered)
	case view.FileBodyDeleted:
		file.Deleted = body.Deleted
	case view.FileBodyDocument:
		file.Document = documentHTML(body.Document)
	case view.FileBodyRows:
		file.Rows = codeRows(body.Diff)
	}
	out := filesView(p, waiting)
	out.Kind, out.File = view.FilesKindFile, &file
	return out
}

// filesView is the top bar a file address's view carries.
func filesView(p page, waiting *int32) view.Files {
	return view.Files{Title: p.Title, Back: p.Back, BackLabel: p.BackLabel, WaitingCount: waiting}
}

// fileBody is what the file page shows the file as, in the order the template
// chooses: a rendering, the deleted box, a document, then the diff's rows or
// the line saying why it has none.
func fileBody(f filePage) view.FileBody {
	switch {
	case f.Rendered != nil:
		return view.FileBodyRendered
	case f.Deleted != "":
		return view.FileBodyDeleted
	case f.ShowDocument:
		return view.FileBodyDocument
	case f.Diff.Binary:
		return view.FileBodyBinary
	case f.Diff.TooLarge:
		return view.FileBodyTooLarge
	}
	return view.FileBodyRows
}

// documentHTML is the one place a rendered document becomes the view's trusted
// field. render.Document's output comes from goldmark with raw HTML off and
// from the word marking, so a test fails on a conversion to view.DocumentHTML
// anywhere else, and the trusted markup keeps one path to the page.
func documentHTML(h template.HTML) view.DocumentHTML {
	return view.DocumentHTML(h)
}

// viewRendering is a rendering as the JSON view holds it.
func viewRendering(r *rendering) *view.Rendering {
	out := &view.Rendering{
		Frame:     r.Frame,
		Sandboxed: r.Sandboxed,
		Title:     r.Title,
		Kind:      r.Kind,
		Size:      r.Size,
	}
	for _, side := range r.Sides {
		out.Sides = append(out.Sides, view.Side{
			Label:   side.Label,
			Tint:    side.Tint,
			Caption: side.Caption,
			Src:     side.Src,
			Element: view.MediaElement(side.Element),
		})
	}
	if r.Facts != nil {
		facts := view.Facts(*r.Facts)
		out.Facts = &facts
	}
	return out
}

// viewNotices is banners as the JSON view holds them.
func viewNotices(notices []notice) []view.Notice {
	var out []view.Notice
	for _, n := range notices {
		out = append(out, view.Notice(n))
	}
	return out
}

// viewNoticePtr is a banner as the JSON view holds it, and nil for none.
func viewNoticePtr(n *notice) *view.Notice {
	if n == nil {
		return nil
	}
	out := view.Notice(*n)
	return &out
}

// The class names render gives a changed line, which codeRows turns into a
// kind.
const (
	classAdd = "add"
	classDel = "del"
)

// tokenRoles maps the role classes render writes to the view's roles. A class
// not listed is plain.
var tokenRoles = map[string]view.TokenRole{
	"k": view.TokenRoleKeyword,
	"s": view.TokenRoleString,
	"c": view.TokenRoleComment,
	"n": view.TokenRoleNumber,
	"f": view.TokenRoleFunction,
}

// codeRows is a file's rows as the JSON view holds them: each line as tokens
// with their roles, so the app does no highlighting of its own.
func codeRows(f render.File) []view.CodeRow {
	var out []view.CodeRow
	for _, row := range f.Rows {
		code := view.CodeRow{
			Kind:      view.CodeRowKindContext,
			Header:    row.Header,
			Number:    int32(row.Number),
			Sign:      row.Sign,
			NoNewline: row.NoNewline,
		}
		switch {
		case row.Header != "":
			code.Kind = view.CodeRowKindHunk
		case row.Class == classAdd:
			code.Kind = view.CodeRowKindAdd
		case row.Class == classDel:
			code.Kind = view.CodeRowKindDel
		}
		for _, seg := range row.Segments {
			role, ok := tokenRoles[seg.Role]
			if !ok {
				role = view.TokenRolePlain
			}
			code.Tokens = append(code.Tokens, view.Token{Text: seg.Text, Role: role, Mark: seg.Mark})
		}
		out = append(out, code)
	}
	return out
}
