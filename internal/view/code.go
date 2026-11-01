package view

// CodeRowKind is what a row of code is.
type CodeRowKind string

// The kinds of code row.
const (
	// CodeRowKindHunk is a hunk's header line, such as "@@ -3,4 +3,6 @@".
	CodeRowKindHunk CodeRowKind = "hunk"
	// CodeRowKindContext is a line that did not change.
	CodeRowKindContext CodeRowKind = "context"
	// CodeRowKindAdd is a line the change added.
	CodeRowKindAdd CodeRowKind = "add"
	// CodeRowKindDel is a line the change removed.
	CodeRowKindDel CodeRowKind = "del"
)

// TokenRole is the syntax role of a run of code. The app maps each role to a
// class and does no highlighting of its own.
type TokenRole string

// The roles a token can have.
const (
	// TokenRolePlain is text no role applies to.
	TokenRolePlain TokenRole = "plain"
	// TokenRoleKeyword is a language keyword.
	TokenRoleKeyword TokenRole = "keyword"
	// TokenRoleString is a string literal.
	TokenRoleString TokenRole = "string"
	// TokenRoleComment is a comment.
	TokenRoleComment TokenRole = "comment"
	// TokenRoleNumber is a number literal.
	TokenRoleNumber TokenRole = "number"
	// TokenRoleFunction is the name of a function where it is declared.
	TokenRoleFunction TokenRole = "function"
)

// CodeRow is one row of a table of code: a hunk header or a line. A file page
// sends its rows in this shape, and so does the review page.
type CodeRow struct {
	// Kind says whether the row is a header or a line, and which way a line
	// changed.
	Kind CodeRowKind `json:"kind"`
	// Header is a hunk header's text, and is empty on a line.
	Header string `json:"header"`
	// Number is the line's number: the old side's on a removed line, the new
	// side's otherwise. It is zero on a header.
	Number int32 `json:"number"`
	// Sign is "+", the minus sign, or empty for a line that did not change.
	Sign string `json:"sign"`
	// Tokens are the line's text, cut where a role or a changed word starts or
	// ends, so joining their text gives the line.
	Tokens []Token `json:"tokens"`
	// NoNewline says the line is the last of a file with no final newline.
	NoNewline bool `json:"no_newline"`
}

// Token is a run of a line with one role and one mark state. A token never
// crosses a role or a mark, so a changed word inside a string is the end of
// one token and the start of the next.
type Token struct {
	// Text is the run's text.
	Text string `json:"text"`
	// Role is the run's syntax role.
	Role TokenRole `json:"role"`
	// Mark says the run is a word that changed, within a line paired with its
	// replacement.
	Mark bool `json:"mark"`
}
