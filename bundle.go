package eyeball

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"path"
	"strconv"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/evanw/esbuild/pkg/api"
)

var (
	bundleOnce sync.Once
	bundled    []byte
	bundleErr  error

	workerOnce sync.Once
	worker     []byte
	workerErr  error
	stamp      string
)

// Bundle is web/ts compiled into one script, built the first time it is asked
// for and kept. Building in the binary keeps the daemon a single file with no
// build step outside go build; it costs esbuild's size in the binary.
func Bundle() ([]byte, error) {
	bundleOnce.Do(func() { bundled, bundleErr = build("web/ts/main.ts") })
	return bundled, bundleErr
}

// Worker is web/ts/sw.ts, the service worker, compiled the same way. It is a
// separate script because a worker's scope is not a page's.
func Worker() ([]byte, error) {
	workerOnce.Do(func() { worker, stamp, workerErr = buildWorker() })
	return worker, workerErr
}

// WorkerStamp names the build of the script Worker returns: a hash of the
// script built with a placeholder in the stamp's place. The worker carries the
// same string and sends it with each tap beacon, so a log line that holds two
// different stamps shows a worker older than the daemon's sw.js.
func WorkerStamp() (string, error) {
	_, err := Worker()
	return stamp, err
}

// stampPlaceholder stands in for the stamp during the build that is hashed. The
// stamp cannot be a hash of a script that holds it, so it is a hash of that
// script before it holds it.
const stampPlaceholder = "unstamped"

// buildWorker builds sw.ts twice. The first pass has the placeholder and is
// hashed. The second has the hash, and is what the daemon serves. The hash
// covers every file sw.ts imports, since esbuild bundles them into the first
// pass.
func buildWorker() ([]byte, string, error) {
	return stamped(func(stamp string) ([]byte, error) {
		return buildWith("web/ts/sw.ts", stamp)
	})
}

// builder builds the worker with the given string as its stamp.
type builder func(stamp string) ([]byte, error)

// stamped runs the two passes of buildWorker over any builder.
func stamped(run builder) ([]byte, string, error) {
	draft, err := run(stampPlaceholder)
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(draft)
	id := hex.EncodeToString(sum[:])[:12]
	out, err := run(id)
	return out, id, err
}

// build runs esbuild over the embedded sources. A warning fails the build as an
// error does, so a source esbuild only half understands is caught by the test
// rather than shipped.
func build(entry string) ([]byte, error) {
	return buildWith(entry, stampPlaceholder)
}

// buildWith is build with the worker's stamp defined as the given string.
func buildWith(entry, workerStamp string) ([]byte, error) {
	res := api.Build(api.BuildOptions{
		Define:      map[string]string{"workerBuild": strconv.Quote(workerStamp)},
		EntryPoints: []string{entry},
		Bundle:      true,
		Write:       false,
		Format:      api.FormatIIFE,
		Target:      api.ES2022,
		Plugins:     []api.Plugin{embedded},
		LogLevel:    api.LogLevelSilent,
	})
	var problems []string
	for _, m := range append(res.Errors, res.Warnings...) {
		problems = append(problems, m.Text)
	}
	if len(problems) > 0 {
		return nil, errors.Newf("bundling %s: %s", entry, strings.Join(problems, "; "))
	}
	if len(res.OutputFiles) != 1 {
		return nil, errors.Newf("bundling %s: %d outputs, want 1", entry, len(res.OutputFiles))
	}
	return res.OutputFiles[0].Contents, nil
}

// embedded resolves and loads imports from Scripts. Imports are written without
// an extension, which tsc requires, so one is added here.
var embedded = api.Plugin{
	Name: "embedded",
	Setup: func(b api.PluginBuild) {
		b.OnResolve(api.OnResolveOptions{Filter: ".*"}, func(a api.OnResolveArgs) (api.OnResolveResult, error) {
			p := a.Path
			if a.Importer != "" {
				p = path.Join(path.Dir(a.Importer), p)
			}
			if !strings.HasSuffix(p, ".ts") {
				p += ".ts"
			}
			return api.OnResolveResult{Path: path.Clean(p), Namespace: "embedded"}, nil
		})
		b.OnLoad(api.OnLoadOptions{Filter: ".*", Namespace: "embedded"}, func(a api.OnLoadArgs) (api.OnLoadResult, error) {
			data, err := fs.ReadFile(Scripts, a.Path)
			if err != nil {
				return api.OnLoadResult{}, err
			}
			contents := string(data)
			return api.OnLoadResult{Contents: &contents, Loader: api.LoaderTS}, nil
		})
	},
}
