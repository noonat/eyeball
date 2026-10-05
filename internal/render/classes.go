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
)

// Classes is every class render chooses at runtime.
var Classes = []string{classAdd, classDel, classMark, roleKeyword, roleString, roleComment, roleNumber, roleFunction}
