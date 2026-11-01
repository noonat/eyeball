package view

// Project is the view of one project: its open reviews, then each of its
// working copies as they are on disk now.
type Project struct {
	// Title is the document's title and the top bar's heading.
	Title string `json:"title"`
	// Back is where the top bar's back link goes.
	Back string `json:"back"`
	// BackLabel is the back link's text.
	BackLabel string `json:"back_label"`
	// WaitingCount is how many reviews wait, which the top bar's eye shows. It
	// is null where the count could not be read, and the eye is then not drawn.
	WaitingCount *int32 `json:"waiting_count"`
	// Name is the project's name.
	Name string `json:"name"`
	// Root is the project's root written with ~.
	Root string `json:"root"`
	// Open counts the open reviews.
	Open int32 `json:"open"`
	// Waiting is the open reviews with a request outstanding, in the queue's
	// order. A section with no rows is sent as null.
	Waiting []Row `json:"waiting"`
	// Working is the open reviews an agent holds, in the queue's order.
	Working []Row `json:"working"`
	// Copies are the project's working copies on disk, the main checkout first.
	Copies []WorkingCopy `json:"copies"`
	// Live says some working copy lists rows, so the screen says once that what
	// it lists is live and read only.
	Live bool `json:"live"`
	// Named says each working copy carries its name and hint: there are
	// several, or the one there is is not at the project's root.
	Named bool `json:"named"`
}

// WorkingCopy is one working copy on the project view.
type WorkingCopy struct {
	// Name is the working copy's directory, and is empty where the project has
	// one working copy.
	Name string `json:"name"`
	// Hint says what the working copy is and where, as "Main checkout,
	// ~/src/harbor". It is empty where the project has one working copy.
	Hint string `json:"hint"`
	// Notices are the banners saying what the working copy cannot show and why.
	Notices []Notice `json:"notices"`
	// Count is the number of files the working copy lists, written out, and
	// empty where nothing is listed.
	Count string `json:"count"`
	// Capped says how many files the rows leave out, and is null where every
	// file has a row.
	Capped *Notice `json:"capped"`
	// Rows are the working copy's files, up to the cap.
	Rows []WorkingRow `json:"rows"`
	// Empty is the box shown in place of the rows, and null where there are
	// rows or no reason to show one.
	Empty *EmptyBox `json:"empty"`
}

// WorkingRow is one changed file in a working copy.
type WorkingRow struct {
	// Path is the file's path.
	Path string `json:"path"`
	// Href is the file's page: the claiming review's file page, or the
	// project's file page.
	Href string `json:"href"`
	// Claimed says an open review covers the file, so the row carries no
	// counts.
	Claimed bool `json:"claimed"`
	// Added is the lines added against HEAD, and is empty for none.
	Added string `json:"added"`
	// Removed is the lines removed against HEAD, and is empty for none.
	Removed string `json:"removed"`
}

// EmptyBox is the box a screen shows in place of rows it has none of.
type EmptyBox struct {
	// Headline is the box's bold line.
	Headline string `json:"headline"`
	// Text is the sentence under it.
	Text string `json:"text"`
}

// Notice is one banner.
type Notice struct {
	// Warn says the banner warns rather than informs.
	Warn bool `json:"warn"`
	// Heading is the banner's bold opening.
	Heading string `json:"heading"`
	// Text is the rest of the banner.
	Text string `json:"text"`
}

// NotFound is what an address with nothing at it answers, with status 404.
type NotFound struct {
	// WaitingCount is how many reviews wait, which the top bar's eye shows. It
	// is null where the count could not be read, and the eye is then not drawn.
	WaitingCount *int32 `json:"waiting_count"`
	// Notice says why a live read was refused, and is null where the path is
	// simply not there.
	Notice *Notice `json:"notice"`
}
