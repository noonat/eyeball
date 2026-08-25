.DEFAULT_GOAL := help

# Tool binaries land in GOPATH/bin. Ask go where that is rather than guessing at
# an install location, so make from a non-interactive shell finds them without a
# profile having been sourced. backlog runs required_commands that way, and a
# gate that only passes in an interactive shell is not a gate.
export PATH := $(shell go env GOPATH)/bin:$(PATH)

# Every Go file that is committed or could be, which is not the same as every
# tracked file: a new package is untracked until it is added, and a gofmt target
# that reads only the index checks nothing at all in a fresh tree and says it
# passed. --exclude-standard keeps ignored build output out.
#
# testdata is excluded because internal/convention's fixtures break conventions
# on purpose, and two of them break formatting to do it: the brace-lines fixture
# is a one-line function body and the argument-wrapping one is a half-wrapped
# call. Formatting them would delete the violation each exists to prove.
#
# `gofmt -l .` would be shorter and is wrong for the same reason. It walks
# testdata, and it also walks .git and ignored build output, which the git list
# leaves out for free.
GOFILES = $(shell git ls-files --cached --others --exclude-standard '*.go' \
	| grep -v '/testdata/')

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

# One tool per domain, no overlap. gofmt owns formatting, vet owns the
# correctness checks the compiler skips, staticcheck owns the rest.
#
# staticcheck runs through `go tool`, so its version comes from go.mod rather
# than from whatever happens to be on the path.
lint: ## Lint everything; fails on any drift, fixes nothing
	@echo "==> gofmt"; \
	  files="$(GOFILES)"; \
	  if [ -z "$$files" ]; then echo "no Go files found; the file list is wrong"; exit 1; fi; \
	  out="$$(gofmt -l $$files)"; \
	  if [ -n "$$out" ]; then echo "gofmt needs a rewrite:"; echo "$$out"; exit 1; fi
	@echo "==> go vet"; go vet ./...
	@echo "==> staticcheck"; go tool staticcheck ./...

fmt: ## Auto-fix what lint checks
	@echo "==> gofmt -w"; gofmt -w $(GOFILES)

check: build lint test ## Build, lint and test: the gate every iteration closes on
	@echo "==> check passed"

.PHONY: help tools build test lint fmt check
