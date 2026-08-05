# Contributing to LuaScript

Thanks for taking the time. LuaScript is a Lua 5.4-flavored language with
Luau-style gradual types, implemented in Go as a clean-room
lex → parse → constcheck → typecheck → optimize → bytecode → stack-VM pipeline.
The goal is an implementation that stays readable end to end, so changes are
judged as much on clarity as on correctness.

By participating you agree to the [Code of Conduct](CODE_OF_CONDUCT.md).

## Getting set up

You need Go (the version in `go.mod`; `go build` will tell you if yours is too
old) and nothing else for the default build.

```sh
git clone https://github.com/HilthonTT/sakura-lang.git
cd sakura-lang
go build ./...
go test ./...
```

Common commands — plain `go`, or the `make` wrappers:

```sh
go run ./cmd/luascript                       # REPL
go run ./cmd/luascript examples/05_types.lsc # run a script
go run ./cmd/luascript -dis examples/01_basics.lsc  # bytecode dump
make check                                   # fmt-check + vet + test
make help                                    # every target
```

Two parts of the tree do **not** build by default, on purpose:

- **`ui`** (Fyne desktop module) is behind `-tags luascript_ui` because it pulls
  in OpenGL via cgo. The default build compiles a headless stub.
- **`plugin`** (loading Go packages at run time) needs cgo on a platform where
  Go supports plugins. It **cannot run on Windows at all**; verify it under WSL:
  `CGO_ENABLED=1 go test ./internal/plugin/`.

## Before you open a pull request

1. `gofmt -l .` reports nothing (`make fmt` fixes it).
2. `go vet ./...` is clean.
3. `go test ./...` passes.
4. Tests cover the change.

CI runs build + vet + test on Linux and Windows, a gofmt check, `go test -race`,
`govulncheck`, and CodeQL. Running `make check` locally catches almost all of it.

## Testing conventions

Tests live next to the code they cover (`*_test.go`). A few suites are worth
knowing about before you touch them:

- `internal/compiler/bytecode` asserts **exact opcode sequences** for
  representative snippets. That is deliberate — it catches codegen drift early.
  If your change moves those sequences, update the expectations consciously and
  say why in the PR; don't edit them just to turn the suite green.
- `internal/compiler/typecheck/checker_test.go` is the focused type-checker suite.
- `internal/vm` holds the benchmarks: `go test ./internal/vm -bench . -benchmem`.
- Fuzz targets exist for the lexer and parser: `make fuzz FUZZ=FuzzParser`.

Some paths CI cannot exercise — the REPL, `httpserver`, bonsai's alt-screen
mode, the `-tags luascript_ui` build, and `plugin`. Smoke-test those by hand and
mention it in the PR.

## Where things live

`CLAUDE.md` in the repo root is the long-form architecture guide: the stage
contracts, the VM invariants, and the gotchas that are load-bearing rather than
incidental. Read the section for the stage you're touching before changing it —
several designs that look accidental are not (e.g. `queue` runs Lua jobs only on
the VM goroutine; `try`/`catch` is a real protected region, not a `pcall`
lowering). If your change invalidates something documented there, update it in
the same PR.

Quick map:

| Path | What it is |
| --- | --- |
| `cmd/luascript/` | CLI entry point, subcommands, bundler, native registration |
| `internal/compiler/` | lexer, parser, AST, constcheck, typecheck, optimize, bytecode |
| `internal/vm/` | stack VM, values, metatables, coroutines, Lua stdlib |
| `internal/native/stdlib/` | requireable native modules |
| `internal/docs/` | single source of truth for stdlib documentation |
| `internal/lsp/`, `client/` | language server and VS Code extension |
| `examples/` | `.lsc` sample programs |

## Adding things

**A native module.** Export a one-line `RegisterX(v *vm.VM)` that calls
`vm.RegisterPreload`, then add **one line** to
`cmd/luascript/natives.go::nativeRegistrars` — that list is the single source of
truth and both code paths (CLI and bundled `.exe`) walk it. Respect the FFI
rule: only `nil`/`bool`/`int64`/`float64`/`string`/`*Table`/`*Closure`/`*GoFunc`
are runtime-tracked, so cast raw Go `int`/`rune`/etc. at the boundary. Use the
argument helpers in `internal/vm/stdlib_args.go` rather than hand-rolled panics.
Avoid Lua reserved words in method names.

**A stdlib function or module member.** Document it in `internal/docs` in the
same PR. `luascript doc -audit` diffs the registry against a live VM and the
`TestDocsMatchRuntime` test fails on drift — and the docs registry is also what
feeds the editor's completion and hover.

**Language syntax.** Add an example under `examples/`, extend the parser and
bytecode tests, and update the README's feature list. Note that several
constructs are desugared in the parser (compound assignment, `match`, backtick
interpolation) and have no dedicated AST node or opcode; prefer that route when
it fits.

## Reporting bugs

Use the issue templates. For a language bug, the single most useful thing is a
**minimal `.lsc` snippet**, the mode directive it ran under (`--!strict` /
`--!nonstrict` / `--!nocheck` / none), and — for anything smelling like codegen
— the output of `-dis`. If stock Lua 5.4 disagrees with us, say what `lua5.4`
does with the same snippet.

Check the README's "Not in v1 (deliberately)" list before filing a type-checker
issue: intersections, string-singleton types, cross-module type checking, and
recursive aliases are known exclusions, not oversights.

For security problems, follow [SECURITY.md](SECURITY.md) instead — do not open a
public issue.

## Commit messages

Conventional-commit prefixes, matching the existing history: `feat:`, `fix:`,
`chore:`, `ci:`, `docs:`, `refactor:`, `test:`, plus `gofmt:` for pure
formatting commits. Subject in the imperative mood, body for the *why*.

## License

Contributions are licensed under the repository's [MIT license](LICENSE).
