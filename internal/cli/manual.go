package cli

import _ "embed"

// manual is what eyeball doc prints. It is markdown, so the prose checks in
// internal/convention read it like any other committed document, and a test
// holds it to every command, flag, refusal code and JSON field.
//
//go:embed manual.md
var manual string
