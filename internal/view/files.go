package view

// FilesKind is what a file address answers: a directory or a file.
type FilesKind string

// The kinds a file address answers.
const (
	// FilesKindTree is a directory, or the project's root.
	FilesKindTree FilesKind = "tree"
	// FilesKindFile is one file.
	FilesKindFile FilesKind = "file"
)

// Files is the view of a review's file address, and of a project's live file.
// One route answers a directory or a file by its path, so the view says which
// and holds the part for it. The other part is null.
type Files struct {
	// Title is the document's title and the top bar's heading.
	Title string `json:"title"`
	// Back is where the top bar's back link goes.
	Back string `json:"back"`
	// BackLabel is the back link's text.
	BackLabel string `json:"back_label"`
	// WaitingCount is how many reviews wait, which the top bar's eye shows. It
	// is null where the count could not be read, and the eye is then not drawn.
	WaitingCount *int32 `json:"waiting_count"`
	// Kind says which of Tree and File is set.
	Kind FilesKind `json:"kind"`
	// Tree is the directory, and is null for a file.
	Tree *Tree `json:"tree"`
	// File is the file, and is null for a directory.
	File *File `json:"file"`
}

// Tree is one directory at one round.
type Tree struct {
	// Dir is the crumb's lead: the project's name and the directory's parent.
	Dir string `json:"dir"`
	// Name is the directory's own name, or the project's at the root.
	Name string `json:"name"`
	// Notices are the banners above the listing, in order.
	Notices []Notice `json:"notices"`
	// Rows are the directory's entries, with the subdirectories that hold a
	// file the review changes expanded in place.
	Rows []TreeRow `json:"rows"`
	// Empty is the sentence the empty box shows where there are no rows.
	Empty string `json:"empty"`
}

// TreeRow is one entry of a directory.
type TreeRow struct {
	// Name is the entry's name.
	Name string `json:"name"`
	// Href is the entry's page.
	Href string `json:"href"`
	// Dir says the entry is a directory.
	Dir bool `json:"dir"`
	// Indent says the entry sits inside an expanded subdirectory.
	Indent bool `json:"indent"`
	// InScope says the entry is a file the review changes, which is bold.
	InScope bool `json:"in_scope"`
	// Added is the review's lines added, and is empty for none.
	Added string `json:"added"`
	// Removed is the review's lines removed, and is empty for none.
	Removed string `json:"removed"`
}

// FileBody is what a file page shows as the file itself.
type FileBody string

// The bodies a file page can have.
const (
	// FileBodyRendered is the file shown as itself: a page in its frame,
	// images, a player, or the line naming a file the surface does not show.
	// Rendered holds it.
	FileBodyRendered FileBody = "rendered"
	// FileBodyDeleted is the box saying the file is gone. Deleted holds its
	// sentence.
	FileBodyDeleted FileBody = "deleted"
	// FileBodyDocument is a markdown file drawn as a document. Document holds
	// it.
	FileBodyDocument FileBody = "document"
	// FileBodyBinary is the line saying the file is binary.
	FileBodyBinary FileBody = "binary"
	// FileBodyTooLarge is the line saying the file is too large to show.
	FileBodyTooLarge FileBody = "too_large"
	// FileBodyRows is the file as rows of code. Rows holds them.
	FileBodyRows FileBody = "rows"
)

// DocumentHTML is a markdown file rendered, with heading ids. It is the one
// field the app inserts as markup, so it has its own type: the server converts
// to it in one function, and a test that type-checks the server's source fails
// on any other conversion to it and on any constant of the type.
type DocumentHTML string

// DocPart is one entry of a rendered document: a passage, or a fold holding a
// run of unchanged passages. Exactly one of Passage and Fold is set.
type DocPart struct {
	// Passage is the entry, and is null for a fold.
	Passage *DocPassage `json:"passage"`
	// Fold is the run of passages folded into one line that opens in place,
	// and is null for a passage.
	Fold []DocPassage `json:"fold"`
}

// DocPassage is one passage of a document: its markup, the source lines it
// covers and whether it changed.
type DocPassage struct {
	// HTML is the passage as the renderer wrote it.
	HTML DocumentHTML `json:"html"`
	// First and Last are the source lines the passage covers, and zero for a
	// run a change removed, which is not in the file and takes no comment.
	First int32 `json:"first"`
	// Last is the passage's last source line.
	Last int32 `json:"last"`
	// Changed says the passage was added or differs from the old side.
	Changed bool `json:"changed"`
	// Thread is the comments on the passage, and is null where it has none or
	// the page shows no comments.
	Thread *Thread `json:"thread"`
}

// File is one file at one round, or one file of a working copy on disk.
type File struct {
	// Dir is the path's directory with a trailing slash, and is empty at the
	// root.
	Dir string `json:"dir"`
	// Name is the path's last segment.
	Name string `json:"name"`
	// Notices are the banners above the file, in order.
	Notices []Notice `json:"notices"`
	// Toolbar is the counts and the control choosing the view, and is null for
	// a file with neither.
	Toolbar *Toolbar `json:"toolbar"`
	// Mode is the line saying the file's mode changed, or that an added file is
	// not a regular one, and is empty otherwise.
	Mode string `json:"mode"`
	// Body says which of the fields below the file is shown by.
	Body FileBody `json:"body"`
	// Rendered is the file shown as itself, and is null unless Body is
	// rendered.
	Rendered *Rendering `json:"rendered"`
	// Deleted is the sentence the empty box shows, and is empty unless Body is
	// deleted.
	Deleted string `json:"deleted"`
	// Passages are the markdown rendered, and are null unless Body is document.
	Passages []DocPart `json:"passages"`
	// Rows are the file's rows of code, and are null unless Body is rows.
	Rows []CodeRow `json:"rows"`
}

// Toolbar is the toolbar above a file that a comparison covers.
type Toolbar struct {
	// Note says why there are no counts and no control, in their place:
	// nothing moved, or the change cannot be measured.
	Note string `json:"note"`
	// Added is the lines added.
	Added string `json:"added"`
	// Removed is the lines removed.
	Removed string `json:"removed"`
	// Counted says what Added and Removed count.
	Counted string `json:"counted"`
	// Marked is a code file's marked view's address, and is empty where the
	// control is not shown.
	Marked string `json:"marked"`
	// Plain is a code file's plain view's address, and is empty where the
	// control is not shown.
	Plain string `json:"plain"`
	// ShowPlain says the plain view is the one shown.
	ShowPlain bool `json:"show_plain"`
	// Rendered is the address of the rendered view of a file with a rendering,
	// markdown or a page, and is empty where the control is not shown.
	Rendered string `json:"rendered"`
	// RenderedLabel names the rendering: Document, Page or Image.
	RenderedLabel string `json:"rendered_label"`
	// Source is the address of the source view of a file with a rendering, and
	// is empty where the control is not shown.
	Source string `json:"source"`
	// ShowSource says the source view is the one shown.
	ShowSource bool `json:"show_source"`
}

// MediaElement is the element that plays or shows one side of a rendering.
type MediaElement string

// The elements a side is drawn with.
const (
	// MediaElementImg is an image.
	MediaElementImg MediaElement = "img"
	// MediaElementAudio is a recording.
	MediaElementAudio MediaElement = "audio"
	// MediaElementVideo is a video.
	MediaElementVideo MediaElement = "video"
)

// Rendering is a file the browser shows itself: frames before over after, one
// frame holding a page or a PDF, or the line naming a file the surface does
// not show.
type Rendering struct {
	// Sides are the images or players, before over after.
	Sides []Side `json:"sides"`
	// Frame is the address of a page or a PDF shown in a frame, and is empty
	// for none.
	Frame string `json:"frame"`
	// Sandboxed says the frame holds a page, which runs under the sandbox.
	Sandboxed bool `json:"sandboxed"`
	// Title names the frame for a screen reader.
	Title string `json:"title"`
	// Kind captions a PDF's frame with what the file is.
	Kind string `json:"kind"`
	// Size captions a PDF's frame with how large it is.
	Size string `json:"size"`
	// Facts is the line naming the file, and is null where it is shown.
	Facts *Facts `json:"facts"`
}

// Side is one image or player.
type Side struct {
	// Label is Before, After, New or Deleted, or the kind's name where nothing
	// is compared.
	Label string `json:"label"`
	// Tint is was or now, matching the diff's palette, and is empty where
	// nothing is compared.
	Tint string `json:"tint"`
	// Caption is the dimensions and size, or the kind's name and size.
	Caption string `json:"caption"`
	// Src is the side's bytes, through the raw route.
	Src string `json:"src"`
	// Element is how the side is drawn.
	Element MediaElement `json:"element"`
}

// Facts is the line naming a file the surface does not show.
type Facts struct {
	// Kind says what the file is.
	Kind string `json:"kind"`
	// Size says how large it is.
	Size string `json:"size"`
	// Hint says why it is not shown, and is empty for a file that has no
	// rendering at all.
	Hint string `json:"hint"`
	// Load is the file's page with it sent anyway, for a file past the send
	// limit, and is empty otherwise.
	Load string `json:"load"`
}
