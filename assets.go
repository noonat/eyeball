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

// Fonts is docs/design/fonts: the faces app.css names by paths relative to
// itself, so the tearouts beside it and the surface's /static/ resolve them the
// same way. Each family's license sits beside its files.
//
//go:embed docs/design/fonts
var Fonts embed.FS

// Templates is web/templates: the shell every address answers with, and the
// head it shares with nothing else.
//
//go:embed web/templates
var Templates embed.FS

// Shell is web/static/shell.css: the page around the app, which a tearout's
// phone frame provides and a real page does not.
//
//go:embed web/static/shell.css
var Shell []byte

// Scripts is web/ts, the client's TypeScript: the modules the app imports and
// the service worker, which Worker compiles.
//
//go:embed web/ts
var Scripts embed.FS

// Icons is web/static/icons, the installed page's icons, which tool/appicon
// renders and commits.
//
//go:embed web/static/icons
var Icons embed.FS

// Sources is web/src, the React app's TypeScript and the stylesheets it
// imports, which App bundles.
//
//go:embed web/src
var Sources embed.FS

// Deps is web/deps/node_modules: the files of each pinned package the app's
// bundle reads, which tool/vendor copies out of web/node_modules. It is not
// under web/vendor, because Go's module zip drops files more than one level
// below a vendor directory and a go install would then find nothing to embed.
//
//go:embed all:web/deps/node_modules
var Deps embed.FS
