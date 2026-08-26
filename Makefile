.DEFAULT_GOAL := help

# Tool binaries live in GOPATH/bin, which a shell with no profile will not have.
export PATH := $(shell go env GOPATH)/bin:$(PATH)

# Committed or committable Go, minus testdata. See docs/conventions.md.
GOFILES = $(shell git ls-files --cached --others --exclude-standard '*.go' \
	| grep -v '/testdata/')

# The same, for what oxfmt formats. HTML is excluded. See docs/conventions.md.
FMTFILES = $(shell git ls-files --cached --others --exclude-standard \
	'*.md' '*.css' '*.json')

OXFMT = npx --yes oxfmt@0.65.0

help: ## Show this help
	@printf "Usage: make <target>\n\nTargets:\n"
	@awk 'BEGIN { FS = ":.*?## " } \
	      /^[a-zA-Z0-9_-]+:.*## / { printf "  \033[36m%-8s\033[0m %s\n", $$1, $$2 }' \
	      $(MAKEFILE_LIST) | sort

tools: ## Install the pinned tool dependencies
	go install tool

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
	  command -v npx >/dev/null || { echo "npx not found. oxfmt needs node"; exit 1; }; \
	  files="$(FMTFILES)"; \
	  if [ -z "$$files" ]; then echo "no formattable files found"; exit 1; fi; \
	  $(OXFMT) --check $$files
	@echo "==> go vet"; go vet ./...
	@echo "==> staticcheck"; go tool staticcheck ./...

fmt: ## Auto-fix what lint checks
	@echo "==> gofmt -w"; gofmt -w $(GOFILES)
	@echo "==> oxfmt --write"; $(OXFMT) --write $(FMTFILES)

check: build lint test ## Build, lint and test: the gate every iteration closes on
	@echo "==> check passed"

.PHONY: help tools build test lint fmt check
