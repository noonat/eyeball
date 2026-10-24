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
	// classHint is small faint text: an HTML block's placeholder.
	classHint = "hint"
	// classOld is the struck text of a passage a change replaced, and
	// classWhole a new passage tinted as a whole.
	classOld   = "old"
	classWhole = "whole"
	// classFold is a run of unchanged passages folded into one line.
	classFold = "fold"
)

// Classes is every class render chooses at runtime.
var Classes = []string{classAdd, classDel, classMark, roleKeyword, roleString, roleComment, roleNumber, roleFunction, classBlk, classChanged, classHint, classOld, classWhole, classFold}
