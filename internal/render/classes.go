package render

// The classes render chooses at runtime. A template prints them through an
// action, which the template-classes check strips, so this table is what a
// test holds to app.css instead.
const (
	// classAdd and classDel are an added and a removed row.
	classAdd = "add"
	classDel = "del"
	// classMark is a changed word within a paired line.
	classMark = "w"
	// The five syntax roles app.css colors.
	roleKeyword  = "k"
	roleString   = "s"
	roleComment  = "c"
	roleNumber   = "n"
	roleFunction = "f"
	// classBlk is a document's block, and classChanged marks one.
	classBlk     = "blk"
	classChanged = "changed"
	// classWas is a removed run with nothing in its place, and classWasText
	// the text of one block it replaced.
	classWas     = "was"
	classWasText = "was-text"
	// classHint is small faint text: a block's cue, and an HTML block's
	// placeholder.
	classHint = "hint"
	// classCueShow and classCueHide are what a tap on a block holding
	// replaced text does, closed and open.
	classCueShow = "cue-show"
	classCueHide = "cue-hide"
)

// Classes is every class render chooses at runtime.
var Classes = []string{classAdd, classDel, classMark, roleKeyword, roleString, roleComment, roleNumber, roleFunction, classBlk, classChanged, classWas, classWasText, classHint, classCueShow, classCueHide}
