package bad

// message is split across lines for width, so a reader has to reassemble it
// before knowing what it says. That is what the check reports.
const message = "a constant the author broke in half " +
	"because the whole of it would not fit the margin"
