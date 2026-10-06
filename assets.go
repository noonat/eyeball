// Package eyeball holds the files the binary embeds: the stylesheet and the
// surface's templates.
//
// It sits at the repository root because go:embed cannot reach a parent
// directory, and the stylesheet lives in docs/design/, where every tearout links
// it. The tearouts and the surface therefore share one file rather than two
// copies that drift. See docs/architecture.md.
package eyeball

import "embed"

// Stylesheet is docs/design/app.css, the visual language the tearouts were
// approved against.
//
//go:embed docs/design/app.css
var Stylesheet []byte

// Templates is web/templates, the surface's pages, seeded from the tearouts.
//
//go:embed web/templates
var Templates embed.FS

// Shell is web/static/shell.css: the page around the app, which a tearout's
// phone frame provides and a real page does not.
//
//go:embed web/static/shell.css
var Shell []byte

// Scripts is web/ts, the client's TypeScript, which Bundle compiles.
//
//go:embed web/ts
var Scripts embed.FS

// Htmx is the vendored htmx, served as it was released. web/vendor/VERSION
// names the release and its sha256, which a test holds the file to.
//
//go:embed web/vendor/htmx.min.js
var Htmx []byte
