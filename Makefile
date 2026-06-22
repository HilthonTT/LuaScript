# LuaScript — Makefile
#
# Thin wrapper over the `go` toolchain. Everything still works with plain `go`
# commands (see CLAUDE.md / README); these targets just bundle the common ones.
#
#   make            # build the binary
#   make run FILE=examples/05_types.lsc
#   make test       # full suite
#   make help       # list every target

# ---- configuration ---------------------------------------------------------

PKG        := ./cmd
BINARY     := luascript
# Windows produces a .exe; detect it so `make run`/`make clean` line up.
ifeq ($(OS),Windows_NT)
	BINARY := luascript.exe
endif

GO         ?= go
GOFLAGS    ?=
# FILE is the script `make run` executes; override on the command line.
FILE       ?= examples/01_basics.lsc
# PKGS/RUN target a single package/test for the focused test targets.
PKGS       ?= ./...
RUN        ?=
# FUZZ selects which fuzz target `make fuzz` exercises (FuzzLexer / FuzzParser).
FUZZ       ?= FuzzParser
FUZZPKG    ?= ./compiler/parser/
FUZZTIME   ?= 30s

.DEFAULT_GOAL := build

# ---- build / run -----------------------------------------------------------

## build: compile the interpreter to ./$(BINARY)
.PHONY: build
build:
	$(GO) build $(GOFLAGS) -o $(BINARY) $(PKG)

## run: run a script (make run FILE=examples/05_types.lsc)
.PHONY: run
run:
	$(GO) run $(PKG) $(FILE)

## repl: start the interactive REPL
.PHONY: repl
repl:
	$(GO) run $(PKG) -i

## install: install the binary into $GOBIN / $GOPATH/bin
.PHONY: install
install:
	$(GO) install $(PKG)

# ---- tests -----------------------------------------------------------------

## test: run the full test suite
.PHONY: test
test:
	$(GO) test $(PKGS)

## test-v: run the full test suite, verbose
.PHONY: test-v
test-v:
	$(GO) test -v $(PKGS)

## test-race: run the suite with the race detector
.PHONY: test-race
test-race:
	$(GO) test -race $(PKGS)

## test-one: run one package/test (make test-one PKGS=./vm/ RUN=TestFoo)
.PHONY: test-one
test-one:
	$(GO) test -v $(PKGS) $(if $(RUN),-run $(RUN),)

## cover: write a coverage profile and print the summary
.PHONY: cover
cover:
	$(GO) test -coverprofile=coverage.out $(PKGS)
	$(GO) tool cover -func=coverage.out | tail -1

## cover-html: open the coverage profile in a browser
.PHONY: cover-html
cover-html: cover
	$(GO) tool cover -html=coverage.out

## bench: run benchmarks (the *_bench_test.go files live under ./vm)
.PHONY: bench
bench:
	$(GO) test -bench=. -benchmem $(if $(filter ./...,$(PKGS)),./vm/,$(PKGS))

## fuzz: run a fuzz target (make fuzz FUZZ=FuzzLexer FUZZPKG=./compiler/lexer/)
.PHONY: fuzz
fuzz:
	$(GO) test -fuzz=$(FUZZ) -fuzztime=$(FUZZTIME) $(FUZZPKG)

# ---- quality ---------------------------------------------------------------

## fmt: format all Go sources
.PHONY: fmt
fmt:
	$(GO) fmt ./...

## fmt-check: fail if any Go source is not gofmt-clean
.PHONY: fmt-check
fmt-check:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "These files are not gofmt-clean:"; echo "$$unformatted"; exit 1; \
	fi

## vet: run go vet
.PHONY: vet
vet:
	$(GO) vet ./...

## tidy: tidy and verify go.mod / go.sum
.PHONY: tidy
tidy:
	$(GO) mod tidy

## check: fmt-check + vet + test (the pre-commit gate)
.PHONY: check
check: fmt-check vet test

# ---- housekeeping ----------------------------------------------------------

## clean: remove build artifacts
.PHONY: clean
clean:
	$(GO) clean
	rm -f $(BINARY) coverage.out

## help: list available targets
.PHONY: help
help:
	@grep -hE '^## ' $(MAKEFILE_LIST) | sed -e 's/## //' | sort | awk -F': ' '{ printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2 }'
