<p align="center">
  <img src="assets/logo.png" alt="luascript logo" width="200">
</p>

# luascript

[![CI](https://github.com/HilthonTT/LuaScript/actions/workflows/ci.yml/badge.svg)](https://github.com/HilthonTT/LuaScript/actions/workflows/ci.yml)
[![CodeQL](https://github.com/HilthonTT/LuaScript/actions/workflows/codeql.yml/badge.svg)](https://github.com/HilthonTT/LuaScript/actions/workflows/codeql.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
![Go](https://img.shields.io/badge/go-1.26-00ADD8?logo=go&logoColor=white)

A Lua-flavored language with a stack-based virtual machine and **Luau-style
gradual types**, written in Go.

The surface syntax tracks **Lua 5.4** closely — same chunks, same scoping rules,
same metatables, coroutines and standard-library shape. Optional type
annotations à la [Luau](https://luau.org) check at compile time and are erased
before bytecode, so the runtime is unchanged. The implementation is a clean-room
rewrite meant to be readable end-to-end: lex → parse → constcheck → typecheck →
constant-fold → bytecode → stack VM. No LLVM, no JIT, no surprises.

> **Docs:** [DESIGN.md](DESIGN.md) for architecture and internals ·
> [examples/](examples/README.md) for the tutorial series ·
> [CONTRIBUTING.md](CONTRIBUTING.md) to contribute

## Quick start

```sh
go run ./cmd/luascript                          # REPL
go run ./cmd/luascript examples/05_types.lsc    # run a script
go build -o luascript ./cmd/luascript           # build a binary
```

```lua
-- closures, multi-return, coroutines: ordinary Lua 5.4
local function counter()
    local n = 0
    return function() n = n + 1; return n end
end

-- ...plus optional types, erased before bytecode
type Point = { x: number, y: number }

local function dist(p: Point): number
    return math.sqrt(p.x * p.x + p.y * p.y)
end

print(dist({ x = 3, y = 4 }))   -- 5.0
```

## CLI

Subcommands are routed before flag parsing:

| Subcommand | What it does |
| ---------- | ------------ |
| `doc [TOPIC]` | Stdlib man pages (alias `man`). Bare = index; `doc math.floor` = one entry; `-k` searches |
| `fmt [-w] FILE` | Trivia-preserving formatter; `-w` writes in place |
| `build -o OUT FILE` | Bundle script + interpreter into one executable |
| `analyze FILE` | AST-level static analysis with pluggable passes |
| `profile -cpu cpu.pgo FILE` | Collect a CPU profile for PGO (`scripts/build-pgo.sh` consumes it) |
| `pkg` | Package manifest / lockfile commands |
| `lsp` | Run the language server on stdio |

| Flag | Effect |
| ---- | ------ |
| `-i` | Force the REPL even when a script is given |
| `-v` | Print version |
| `-dis` | Disassemble to a bytecode dump |
| `-time` / `-watch` | Time the run / re-run on every save (mutually exclusive) |
| `-gc-percent N` / `-mem-limit N` | Host GC knobs (GOGC, soft heap limit) |
| `-bonsai` | Grow an ASCII bonsai tree ([side mode](#bonsai-mode)) |

## Language

Everything Lua 5.4 has — `goto`/labels, metatables, coroutines, varargs, generic
and numeric `for`, `<const>`/`<close>` attributes, the full Lua pattern surface
(`find`/`match`/`gmatch`/`gsub`) — plus:

- **Gradual types** — primitives, function types, optionals (`T?`), unions
  (`A | B`), aliases including structural tables, assertions (`x :: T`).
  Untyped code is `any`; the stdlib has hand-written signatures, so
  `math.sqrt(true)` is a compile error.
- **Generics** — `<T, U>` on functions, aliases and structs, with call-site
  inference. `type Box<T>`, `struct Pair<A, B>`, `Box<Box<number>>`.
- **Refinements** — type guards, nil guards, truthiness, `assert()`, early-exit
  narrowing, short-circuit RHS narrowing.
- **Structs** — `struct Point { x: number, y: number }`: a nominal product type
  with positional (`Point(1, 2)`) and named (`Point{ x = 1 }`) construction.
- **Enums and tagged unions** — `enum Color RED, GREEN end` is a frozen
  int-auto-increment table; give a variant a payload
  (`enum Shape Circle(number), Unit end`) and it becomes a sum type with
  constructors, singletons, `__tag` and `typeof`.
- **`match`** — value/literal arms, typed bindings (`n: number ->`),
  destructuring of enums and structs, and `if` guards.
- **`try` / `catch` / `throw`** — a real protected region in the enclosing
  frame, so `return`/`break`/`continue` inside a `try` act on the enclosing
  function or loop. Not a `pcall` desugar.
- **`defer`** — LIFO cleanup on normal return *and* error unwinding. Captures by
  upvalue, so a deferred call sees the value at exit time.
- **`continue`** — a real statement that closes upvalues on the way out.
- **If expressions** — `if c then a else z`, no `end`, `else` mandatory.
- **Default parameters** — `function f(x: number = 10)`; `false` does not
  trigger the default.
- **String interpolation** — `` `hello {name}` ``.

Mode directives on line 1 set strictness: `--!strict` (implicit-any params
become errors), `--!nonstrict` (the default, stated explicitly), `--!nocheck`
(skip the type pass — but **not** `constcheck`).

Deliberately out of v1: intersections (`A & B`), string-singleton types,
cross-module type checking (`require()` returns `any`), and recursive aliases.
See [DESIGN.md](DESIGN.md#deliberately-out).

## Errors

Runtime errors carry the file and line they were raised at, and an uncaught one
prints the call stack that led there — across module boundaries, since each
chunk is named after the file it was loaded from.

```
$ luascript app.lsc
luascript: lib/parse.lsc:14: attempt to index a nil value
stack traceback:
	lib/parse.lsc:14: in function 'Parser.field'
	lib/parse.lsc:31: in function 'Parser.record'
	app.lsc:8: in main chunk
```

`pcall`, `try`/`catch` and `coroutine.resume` report the position the error was
*raised* at, not the one that caught it. `require("debug")` exposes the same
walk as `debug.traceback([msg [, level]])`.

## Modules

`require("…")` for `db`, `os`, `math`, `json`, `http`, `httpserver`, `crypto`,
`time`, `regexp`, `uuid`, `sort`, `compression`, `bit32`, `utf8`, `io`, `log`,
`debug`, `queue`, and `std` (stack/queue/deque/set/list/heap/hashmap/trie/btree).

Data science: `stats`, `linalg`, `csv`, `dataframe`, `ndarray` (dense N-D arrays
with broadcasting and overloaded operators), `plot` (dependency-free SVG
charting), `clustering`, `classification`, and `ml` (feed-forward neural nets).

All ship by default — `cmd/luascript/natives.go::nativeRegistrars` is the single
source of truth, and adding a module is one line. Two are gated:

- **`ui`** (Fyne desktop GUI) is opt-in behind `-tags luascript_ui`, because
  Fyne pulls in OpenGL via cgo. Without the tag `require("ui")` still resolves;
  it errors only if a script constructs a widget. The tagged build needs cgo, a
  C toolchain and OpenGL headers.
- **`plugin`** (load Go packages at run time) needs cgo and a platform where Go
  supports plugins, so it **never runs on Windows** — `require("plugin")`
  resolves everywhere but reports `supported = false` there.

### Jobs and channels

`require("queue")` gives a priority job queue and Go-backed channels:

```lua
local queue = require("queue")
local q = queue.new{ capacity = 1000 }

q:push(function() cleanup() end)                                    -- priority 0
q:push(function() page_oncall() end, { priority = 100 })            -- runs first
q:push(sync, { retries = 3, backoff_ms = 250, id = "sync-users" })
q:push(reap, { delay_ms = 5000 })
q:run()   -- drains on this goroutine; returns how many jobs ran

local ch = queue.channel(16)     -- 0 (default) is unbuffered
ch:send("work")                  -- ch:send(v, 250) times out
local v, ok = ch:receive(1000)   -- -> value, true | nil, false, "timeout"|"closed"
```

**The one rule: jobs always run on the VM goroutine, one at a time.** The VM has
no locks, so running Lua on two goroutines is a data race, not a speedup. The
queue buys ordering, delays, retries, backpressure, deadline-shedding and
metrics — *not* parallelism. Consequently `timeout_ms` is a deadline on
*starting*: a job past its deadline is dropped unrun, and a job already in
flight cannot be preempted. See
[DESIGN.md](DESIGN.md#concurrency-the-one-rule) and
[`54_queue_module.lsc`](examples/54_queue_module.lsc).

## Bundling a script into an executable

```sh
go build -o luascript ./cmd/luascript
./luascript build -o hello.exe examples/01_basics.lsc
./hello.exe                # runs the embedded script
```

The script is appended to a copy of the interpreter with a magic trailer; on
startup the bundled binary inspects its own tail and runs the embedded source
with the same VM and native modules. Syntax is checked at bundle time.

v1 limits: host platform only (no cross-compilation flag), bundled scripts don't
see `os.Args`, and antivirus heuristics occasionally flag self-appending
executables — the same trade-off PyInstaller and Bun's `--compile` carry.

## REPL

| Command | Effect |
| ------- | ------ |
| `help` | Print the help screen |
| `exit`, `quit` | Leave |
| `reset` | Rebuild the VM (clears globals and user state) |
| `clear` | Clear the screen |
| `doc <topic>` | Stdlib reference, same data as `luascript doc` |

**Ctrl+C** cancels input, **Ctrl+D** exits, **Ctrl+R** searches history. Bare
expressions print their value. Incomplete input opens a continuation prompt, and
type errors get a distinct `type-error:` prefix.

Top-level `local` persists across REPL chunks — it is promoted to a global at
compile time so later inputs can read it. Inside any nested scope (`do`/`if`/
`for`/function body) `local` keeps standard Lua semantics.

## Bonsai mode

An ASCII-bonsai grower, unrelated to the Lua runtime — just a fun side mode.

```sh
./luascript -bonsai                      # alt-screen (q or Ctrl+C to leave)
./luascript -bonsai -bonsai-print        # print one tree to stdout
./luascript -bonsai -bonsai-live         # animate growth
./luascript -bonsai -seed 42             # reproducible
./luascript -bonsai -bonsai-msg "hello"  # attach a message
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for setup, testing conventions, how to
add a native module or stdlib entry, and commit-message style. Before sending a
change:

```sh
make check     # gofmt -l . + go vet ./... + go test ./...
```

Tests live next to the code (`*_test.go`). The bytecode tests assert exact
opcode sequences for representative snippets, so they catch codegen drift early.

Participation is governed by the [Code of Conduct](CODE_OF_CONDUCT.md). Report
security problems via [SECURITY.md](SECURITY.md), not a public issue.

## Inspirations

**Lua 5.4** (syntax and semantics target) · **Luau** (type-system shape) ·
**Goby** (the original stack VM and bytecode-generator scaffolding — this
project is a Goby fork in spirit, though much has been rewritten).

## License

MIT — see [LICENSE](LICENSE).
