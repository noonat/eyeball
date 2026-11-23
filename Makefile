.DEFAULT_GOAL := help

# Tool binaries live in GOPATH/bin, which a shell with no profile will not have.
export PATH := $(shell go env GOPATH)/bin:$(PATH)

# Committed or committable Go, minus testdata. See docs/conventions.md.
GOFILES = $(shell git ls-files --cached --others --exclude-standard '*.go' \
	| grep -v '/testdata/')

# The same, for what oxfmt formats. HTML is excluded, and so is web/deps/, whose
# files must stay byte for byte the release they name, and
# web/src/fixtures/scenes.ts, web/src/fixtures/_raw/ and the review views,
# which tool/fixtures writes and make api-check compares byte for byte. A view
# with a short array, a note of one paragraph, is written as the server sends
# it and oxfmt would put that array on one line. See docs/conventions.md.
FMTFILES = $(shell git ls-files --cached --others --exclude-standard \
	'*.md' '*.css' '*.json' '*.ts' '*.tsx' | grep -v -e '^web/deps/' \
	| grep -v -e '^web/package-lock.json$$' -e '^web/src/fixtures/scenes.ts$$' -e '^web/src/fixtures/_raw/' \
	| grep -v -e '^web/src/fixtures/review-.*\.json$$')

# The JavaScript tools are pinned once, in web/package.json and its lockfile,
# and run from the node_modules `npm ci` installs there. Nothing is fetched at
# lint time, so a lint run needs no network.
WEB = web
BIN = $(WEB)/node_modules/.bin
OXFMT = $(BIN)/oxfmt
TSC = $(BIN)/tsc
# oxlint looks for tsgolint beside the directory it runs from, which is the
# repository root here and not web/.
OXLINT = OXLINT_TSGOLINT_PATH=$(CURDIR)/$(BIN)/tsgolint $(BIN)/oxlint

# Fails with the fix when the tools are not installed, rather than with the
# shell's "not found".
NODE_MODULES_CHECK = test -x $(BIN)/tsc || { echo "web/node_modules is missing. Run: make tools"; exit 1; }

help: ## Show this help
	@printf "Usage: make <target>\n\nTargets:\n"
	@awk 'BEGIN { FS = ":.*?## " } \
	      /^[a-zA-Z0-9_-]+:.*## / { printf "  \033[36m%-8s\033[0m %s\n", $$1, $$2 }' \
	      $(MAKEFILE_LIST) | sort

tools: ## Install the pinned tool dependencies
	go install tool
	cd $(WEB) && npm ci --ignore-scripts --no-audit --no-fund

build: ## Build every package
	go build ./...

test: ## Run the tests
	go test ./...

# staticcheck runs through `go tool`, so go.mod pins its version.
lint: ## Lint everything; fails on any drift, fixes nothing
	@echo "==> gofmt"; \
	  files="$(GOFILES)"; \
	  if [ -z "$$files" ]; then echo "no Go files found; the file list is wrong"; exit 1; fi; \
	  out="$$(gofmt -l $$files)"; \
	  if [ -n "$$out" ]; then echo "gofmt needs a rewrite:"; echo "$$out"; exit 1; fi
	@echo "==> oxfmt"; \
	  command -v node >/dev/null || { echo "node not found. oxfmt needs node"; exit 1; }; \
	  $(NODE_MODULES_CHECK); \
	  files="$(FMTFILES)"; \
	  if [ -z "$$files" ]; then echo "no formattable files found"; exit 1; fi; \
	  $(OXFMT) --check $$files
	@echo "==> tsc"; $(NODE_MODULES_CHECK); \
	  $(TSC) -p web/tsconfig.json && $(TSC) -p web/tsconfig.sw.json && $(TSC) -p web/tsconfig.app.json && $(TSC) -p web/tsconfig.stories.json
	@echo "==> oxlint"; \
	  $(OXLINT) --type-aware --tsconfig web/tsconfig.json --deny-warnings --ignore-pattern sw.ts web/ts && \
	  $(OXLINT) --type-aware --tsconfig web/tsconfig.sw.json --deny-warnings web/ts/sw.ts && \
	  $(OXLINT) --type-aware --tsconfig web/tsconfig.stories.json --deny-warnings web/src web/.storybook web/vite.config.ts web/vitest.config.ts
	@echo "==> go vet"; go vet ./...
	@echo "==> staticcheck"; go tool staticcheck ./...

# Both halves matter. The tests run each story in dev mode, and only the build
# shows a fault of the production bundle: it adds an export to every story
# module that the sheet once drew as a story.
stories: ## Run every story as a test in headless Chromium, then build Storybook
	@echo "==> stories"; $(NODE_MODULES_CHECK); \
	  test -x "$${EYEBALL_CHROMIUM:-/usr/bin/chromium}" || { echo "Chromium not found. Set EYEBALL_CHROMIUM to its path"; exit 1; }; \
	  cd $(WEB) && npm run --silent test-stories && npm run --silent build-storybook >/dev/null

vendor: ## Copy the files the app's bundle reads into web/deps/node_modules
	@$(NODE_MODULES_CHECK); go run ./tool/vendor

# The set follows the app's imports, so a lockfile check would miss an import
# that needs a file that is not vendored yet. This re-runs the step instead.
vendor-check: ## Fail when web/deps/node_modules differs from a fresh vendor step
	@echo "==> vendor"; $(NODE_MODULES_CHECK); go run ./tool/vendor --check

api: ## Write web/src/api/types.ts and web/src/fixtures from internal/view
	go run ./tool/apitypes && go run ./tool/fixtures

# The generated types and the fixtures are views of the Go structs and the
# server, so a change to either has to be committed with the files it moves.
api-check: ## Fail when the generated types or the fixtures differ from a fresh run
	@echo "==> api"; go run ./tool/apitypes --check && go run ./tool/fixtures --check

fmt: ## Auto-fix what lint checks
	@echo "==> gofmt -w"; gofmt -w $(GOFILES)
	@echo "==> oxfmt --write"; $(NODE_MODULES_CHECK); $(OXFMT) --write $(FMTFILES)

check: build lint stories vendor-check api-check test ## Build, lint and test: the gate every iteration closes on
	@echo "==> check passed"

.PHONY: help tools build test lint stories vendor vendor-check api api-check fmt check
