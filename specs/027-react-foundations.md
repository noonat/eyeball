---
status: ready
required_commands:
  - cmd: make check
  - cmd: go test -race ./...
  - cmd: go test -count=1 -tags browser ./internal/server
---

# 027 The React foundations

The surface is moving from htmx and Go templates to React, as
[docs/architecture.md](../docs/architecture.md) describes under The surface and
The client code. This spec builds what every later screen stands on: the Node
toolchain for development, the strictest TypeScript over the app, the vendored
packages and the Go bundle that keep `go install` working, and the foundation
components as Storybook stories, which are their tearout
([docs/design/README.md](../docs/design/README.md)). No screen moves here.

**The shell, the router, the service worker's shell cache and the tap handling
move to the queue spec** (the JSON API and the queue, 028). A shell with no
screen behind it cannot be judged on the phone, and the two problems of the
mixed period appear only once a React screen sits beside templates. While
screens are mixed, the worker must not answer a template page's navigation with
the shell. A tap posted to a template window reaches nothing, so the tap needs
an answer from the app within a short timeout and `navigate` as the fallback,
with the beacons logging which path ran. Both are named here so that spec starts
with them.

**Foundation components are the ones whose props are local**: the shell's top
bar, type, the palette in both themes, icons, the eye, state pills, level chips,
buttons, notices, the empty state, the first-run steps and the icon badge. A
component shaped by server data, such as a queue row, a diff row with role
tokens, a prose passage or a comment card, is built in the first spec that
builds its view, so its props come from the API's generated types rather than a
guess made here. `foundations.html` stays until every primitive it shows has an
approved story.

**`app.css` stays one file through the mixed period.** The templates and the
components both use it, so the two cannot drift while both exist. It splits into
one stylesheet per component when the last template goes, in the cleanup spec.

Measured 2026-10-29, in scratch projects:

- React 19.3.1, React Router 7.18.5 and TanStack Query 5.105.0 bundled by
  esbuild 0.28.2 through its Go API. Copying only the 68 files esbuild's
  metafile lists, plus each package's `package.json`, into a fresh directory and
  bundling from there gave a bundle identical byte for byte to the one built
  from `node_modules`: 353 KB minified, 111 KB gzipped, from 1.4 MB of vendored
  files out of 21 MB installed. The packages resolve through `exports` maps, so
  the binary bundles with esbuild's own resolver over a directory, not through a
  resolve plugin.
- React Router's `exports` reach only its `dist/development` build. The
  production one is not exported.
- TypeScript 7.0.2 with every flag in architecture.md and `skipLibCheck: false`
  passed over a file using all four packages, and failed on a planted
  `exactOptionalPropertyTypes` violation.
- Storybook 10.6.5 with Vite 8.3.5 built a story, and its full-screen address,
  `iframe.html?id=<story>&viewMode=story`, rendered the component alone in
  headless Chromium. Storybook 9 was the version assumed when architecture.md
  was written. 10 is current.
- Storybook's Vitest add-on 10.6.5 with Vitest 4.1.12 ran two stories as tests
  in headless `/usr/bin/chromium`, through Playwright's `executablePath`. One
  passed and a planted one failed. Vite had to be told to pre-bundle React
  (`optimizeDeps.include`), or the add-on's setup file failed to import it.
- esbuild in Go, given an output directory, wrote a CSS import as a second
  output file, `app.css`, with a font `url()` marked external left as it was.

Not probed: Storybook's accessibility add-on run as a test, and Storybook served
from `tool/tearout` to the phone.

## Review

Spec review pass: requested, because the spec holds pairs that must agree with
nothing yet checking them: the vendored files against the app's imports, Vite's
configuration against esbuild's, and the strict flags against the libraries'
types. End-of-branch review pass: declined, because the review agent reads each
iteration and every check this spec adds is proved by a planted violation.

## Out of scope

- The shell, the router, the shell cache and the tap handling, in the queue spec
  (028).
- The JSON API and its generated types, in the same spec.
- Components shaped by server data, in the spec that builds each one's view.
- Splitting `app.css`, in the cleanup spec.
- Vite's dev server proxying to a daemon, which needs an API to proxy.

## Iteration 1: The documents

- [ ] architecture.md says the binary writes the embedded vendor tree to a
      temporary directory once and bundles with esbuild's resolver over it.
- [ ] architecture.md says the vendor set is what esbuild's metafile lists, and
      that the check re-runs the vendor step and compares, since a lockfile
      check misses an import that needs a new file.
- [ ] architecture.md replaces the React 19.1.1 measurement with this spec's,
      and names Storybook 10.
- [ ] architecture.md says `app.css` stays one file until the last template
      goes.
- [ ] specs/TODO.md moves the shell, router, shell cache and tap handling from
      the foundations item to the queue item, naming the two mixed-period
      problems.

## Iteration 2: The tearout

```punchcard
required_acks:
  - tearout-approved
```

- [ ] `web/package.json` and its lockfile pin React, its types, Vite, Storybook,
      its Vitest and accessibility add-ons, Vitest, Playwright, TypeScript,
      oxlint and oxfmt. React Router and TanStack Query are pinned by the queue
      spec, which first imports them. `make tools` runs `npm ci` there.
- [ ] `web/node_modules/` and Storybook's build output are ignored by git, and
      oxfmt formats `.tsx` files as it does `.ts`.
- [ ] `web/tsconfig.app.json` checks `web/src` with every flag in
      architecture.md, `jsx: react-jsx` and `skipLibCheck: false`, and
      `make lint` runs it.
- [ ] `make lint` runs oxlint over `web/src` with its React and React hooks
      rules and no explicit `any`, and fails on a planted violation of each.
- [ ] Storybook's preview loads `docs/design/app.css` and its fonts, and a
      toolbar control or story parameter stamps `data-theme` light or dark.
- [ ] Each foundation component named above is a component in
      `web/src/components/` with a story for every state `foundations.html`
      draws of it.
- [ ] A sheet story lays every foundation state out in 390 by 800 frames, each
      light frame beside its dark one, every frame a link to its state's
      full-screen story.
- [ ] `make check` runs every story as a test in headless Chromium, with its
      interaction test where it has one and the accessibility audit, and fails
      on a planted failing story.
- [ ] A check in `internal/convention` fails when a class a component names in a
      string literal is defined by no stylesheet, proved by a planted one.
- [ ] `go run ./tool/tearout` builds Storybook and serves it over the tailnet
      with `Cache-Control: no-store`, beside the static sheets it serves now.
- [ ] The owner judges the sheet on the phone against `foundations.html`, and
      every change asked for is made before the ack.

## Iteration 3: The Go bundle

- [ ] `tool/vendor` bundles `web/src/main.tsx` from `web/node_modules`, and
      copies each file the metafile lists and each package's `package.json` to
      `web/vendor/node_modules/`. `make vendor` runs it.
- [ ] `make check` re-runs the vendor step into a temporary directory and fails
      when it differs from `web/vendor/node_modules/`, proved by deleting one
      vendored file and by adding an import that needs a new one.
- [ ] `web/src/main.tsx` renders a foundation component into the page, as the
      entry the vendor step and the bundle start from until the queue spec gives
      it the router.
- [ ] `eyeball.App()` writes the embedded vendor tree to a temporary directory
      once per process and bundles `web/src/main.tsx` from it into a script and
      a stylesheet, with `/static/fonts/` left external.
- [ ] A browser test loads the bundle `eyeball.App()` returns into a test page
      and finds a foundation component rendered by it, and fails with the
      vendored React removed.
- [ ] architecture.md's binary layout and `assets.go` list `web/src` and
      `web/vendor/node_modules`, and a binary built with `web/node_modules`
      moved away bundles the app, since that is the build `go install` makes.
