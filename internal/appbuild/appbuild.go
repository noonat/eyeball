package appbuild

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/evanw/esbuild/pkg/api"
)

// Entry is the app's entry point, relative to the root a build runs in.
const Entry = "web/src/main.tsx"

// Result is what one build produced.
type Result struct {
	// Script is the bundle, minified, to be loaded as a classic script.
	Script []byte
	// Stylesheet is the CSS the entry's imports collected. A font url in it is
	// left as written, to be served beside the stylesheet.
	Stylesheet []byte
	// Inputs are the files the bundle read, relative to the build's root.
	Inputs []string
}

// Build bundles Entry from a root directory. The packages it imports resolve
// from each of nodePaths, and then from a node_modules directory above the
// source, which is how the vendor step finds web/node_modules.
//
// A warning fails the build as an error does, so a source esbuild only half
// understands is caught by a test rather than shipped.
func Build(root string, nodePaths []string) (Result, error) {
	res := api.Build(api.BuildOptions{
		AbsWorkingDir: root,
		EntryPoints:   []string{Entry},
		Outdir:        filepath.Join(root, "out"),
		NodePaths:     nodePaths,
		Bundle:        true,
		Write:         false,
		Metafile:      true,
		Format:        api.FormatIIFE,
		Target:        api.ES2022,
		JSX:           api.JSXAutomatic,
		Conditions:    []string{"production"},
		// React reads this to choose its production build, and without it the
		// bundle holds both.
		Define: map[string]string{"process.env.NODE_ENV": `"production"`},
		// app.css names its fonts relative to itself, and the surface serves
		// them beside the stylesheet under /static/fonts/. Neither spelling is a
		// file in the bundle's tree.
		External:          []string{"/static/fonts/*", "fonts/*"},
		TsconfigRaw:       `{"compilerOptions":{"verbatimModuleSyntax":true}}`,
		MinifyWhitespace:  true,
		MinifyIdentifiers: true,
		MinifySyntax:      true,
		LogLevel:          api.LogLevelSilent,
	})
	var problems []string
	for _, m := range append(res.Errors, res.Warnings...) {
		problems = append(problems, m.Text)
	}
	if len(problems) > 0 {
		return Result{}, errors.Newf("bundling %s: %s", Entry, strings.Join(problems, "; "))
	}
	var out Result
	for _, file := range res.OutputFiles {
		switch filepath.Ext(file.Path) {
		case ".js":
			out.Script = file.Contents
		case ".css":
			out.Stylesheet = file.Contents
		}
	}
	if out.Script == nil {
		return Result{}, errors.Newf("bundling %s: no script in %d outputs", Entry, len(res.OutputFiles))
	}
	var meta struct {
		Inputs map[string]json.RawMessage
	}
	if err := json.Unmarshal([]byte(res.Metafile), &meta); err != nil {
		return Result{}, errors.Wrap(err, "read esbuild metafile")
	}
	for name := range meta.Inputs {
		out.Inputs = append(out.Inputs, name)
	}
	sort.Strings(out.Inputs)
	return out, nil
}
