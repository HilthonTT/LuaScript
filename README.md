<p align="center">
  <img src="assets/logo.png" alt="luascript logo" width="200">
</p>

# luascript

A Lua-flavored language with a stack-based virtual machine and **Luau-style gradual types**, written in Go.

The surface syntax tracks **Lua 5.4** as closely as possible — the same chunks, the same scoping rules, the same metatables, coroutines, and standard library shape. Optional type annotations on top, à la [Luau](https://luau.org), check at compile time and erase before bytecode so the runtime is unchanged.

The implementation is a clean-room rewrite focused on being readable end-to-end: lex → parse → typecheck → bytecode → stack VM. No LLVM, no JIT, no surprises.

## Contents

- [Status](#status)
- [Quick start](#quick-start)
- [Bundling a script into a standalone .exe](#bundling-a-script-into-a-standalone-exe)
- [Desktop UI module (opt-in)](#desktop-ui-module-opt-in)
- [Bonsai mode](#bonsai-mode)
- [Examples](#examples)
- [Type system](#type-system)
- [REPL](#repl)
- [Project layout](#project-layout)
- [Non-goals (for now)](#non-goals-for-now)
- [Contributing](#contributing)
- [Inspirations](#inspirations)
- [License](#license)

## Status

- **Lexer** — Lua 5.4 tokens, long-bracket strings/comments, hex/exponent numbers, `--!strict` / `--!nonstrict` / `--!nocheck` mode directives.
- **Parser** — full Lua 5.4 grammar including `goto`/labels, attributes (`<const>`, `<close>`), method-call sugar, numeric and generic `for`, plus Luau type-syntax (annotations, type aliases, type assertions, optionals, unions, function types, structural table types).
- **Type checker** — gradual: untyped code is implicitly `any`; annotations opt in. Primitives, function types, optionals, unions, type aliases (including structural table shapes), type assertions. Stdlib has hand-written signatures so `math.sqrt(true)` is a compile error.
- **Bytecode** — stack-based with closure upvalues, vararg passing, generic-`for` iteration, and a one-time scan that fills `NumLocals` at runtime where the generator left it blank. Types are erased before this stage — the VM never sees them.
- **VM** — closures, metatables, coroutines (via goroutines + channels), `pcall`/`error` unwinding.
- **Standard library** — `print`/`tostring`/`tonumber`, `ipairs`/`pairs`/`next`, `pcall`/`assert`/`error`, `type` plus the `typeof`/`sizeof` reflection builtins, raw and metatable helpers, plus `math`, `string` (full Lua pattern surface: `find`/`match`/`gmatch`/`gsub`), `table`, `io.write`/`read`, `coroutine`, and `package`/`require`. `__tostring` is honoured by `tostring`, `print`, `io.write`, `error`, and the REPL.
- **Native modules** — `require("…")` for `db`, `os`, `math`, `json`, `http` (client), `httpserver`, `crypto`, `time`, `regexp`, `uuid`, `sort`, `compression`, `bit32`, `utf8`, `io`, `log`, `debug`, `std` (stack/queue/deque/set/list/heap/hashmap), `clustering` (k-means/DBSCAN/hierarchical/mean-shift), `classification` (Naive Bayes/KNN/perceptron/logistic/SVM), and the data-science set: `stats` (descriptive/inferential statistics), `linalg` (vectors/matrices), `csv` (read/write), and `dataframe` (column-oriented tables). All ship by default; `cmd/natives.go::nativeRegistrars` is the single source of truth. The Fyne-backed `ui` GUI module is **opt-in** behind the `luascript_ui` build tag (it pulls in OpenGL/cgo) — see [Desktop UI module](#desktop-ui-module-opt-in).
- **Enums** — `enum Name V1, V2 end` declares an int-auto-increment, frozen-via-`__newindex`-proxy table. Lowered at parse time; typecheck treats the alias as `number`.
- **Defer** — `defer cleanup()` schedules a call to run when the enclosing function exits, in last-in-first-out order, on normal return **and** when an error unwinds the frame (caught by `pcall`). Lowered to a frame-local closure list; ideal for paired acquire/release. Capture is by upvalue, so a deferred call sees a variable's value at exit time (unlike Go, which snapshots arguments eagerly).
- **REPL** — readline-driven, history-backed, with continuation prompts for incomplete input. Top-level `local` declarations persist across REPL chunks (a deliberate convenience deviation from `lua`). Type-check errors are surfaced with a distinct `type-error:` prefix.

## Quick start

The `main` package lives in `./cmd`, so run the interpreter with `go run ./cmd`:

```sh
# Run the REPL
go run ./cmd

# Run a script
go run ./cmd examples/05_types.lsc

# Force the REPL even when a script is supplied
go run ./cmd -i examples/05_types.lsc

# Print version
go run ./cmd -v

# Disassemble a script (bytecode dump)
go run ./cmd -dis examples/01_basics.lsc

# Time the run, or re-run on every save
go run ./cmd -time  examples/02_functions.lsc
go run ./cmd -watch examples/02_functions.lsc
```

Build a binary:

```sh
go build -o luascript ./cmd
./luascript examples/01_basics.lsc
```

### Subcommands

The CLI dispatches a few subcommands before flag parsing:

| Subcommand                                            | What it does                                                                |
| ----------------------------------------------------- | --------------------------------------------------------------------------- |
| `luascript fmt [-w] FILE.lsc`                         | Format a source file (trivia-preserving). `-w` writes in place.             |
| `luascript build -o OUT.exe FILE.lsc`                 | Bundle script + interpreter into a single .exe (see next section).          |
| `luascript analyze FILE.lsc`                          | AST-level static analyzer with pluggable passes (complexity, lint, …).      |
| `luascript profile -cpu cpu.pgo -count 50 FILE.lsc`   | Collect a CPU profile suitable for PGO (`scripts/build-pgo.sh` consumes it).|

## Bundling a script into a standalone .exe

`luascript build` produces a single executable that contains both the interpreter and your script — drop it on a machine that doesn't have `luascript` installed and double-click it.

```sh
# Build luascript first, then have it bundle your script:
go build -o luascript ./cmd
./luascript build -o hello.exe examples/01_basics.lsc
./hello.exe                # runs the embedded script
```

| Flag      | Effect                                         |
| --------- | ---------------------------------------------- |
| `-o PATH` | Output path for the bundled binary (required). |

Mechanics: the script is appended to a copy of the `luascript` binary along with a magic trailer. On startup the bundled .exe inspects its own tail, detects the trailer, and runs the embedded script in `parser.NormalMode` with the same VM and native modules the interpreter uses. Syntax is checked at bundle time, so you can't ship a broken .exe by accident.

Limitations (v1):

- The bundled binary matches the host platform — no cross-compilation flag yet.
- Bundled scripts don't see `os.Args`.
- Antivirus heuristics occasionally flag self-modifying-style .exes; code-signing fixes it. This is the same trade-off PyInstaller and Bun's `--compile` have.

## Desktop UI module (opt-in)

The `ui` native module is a thin Lua binding over [Fyne v2](https://fyne.io) for building desktop windows and widgets. Because Fyne drags in OpenGL via **cgo**, it is **not compiled by default** — a plain `go run ./cmd` stays pure-Go and needs no C toolchain. `require("ui")` still resolves in a default build; it only errors if a script actually constructs a widget, telling you to rebuild with the tag.

To enable the real GUI, build or run with the `luascript_ui` build tag:

```sh
# Run a UI script with the Fyne backend compiled in
go run -tags luascript_ui ./cmd examples/31_ui_module.lsc

# Build a GUI-capable binary
go build -tags luascript_ui -o luascript ./cmd
./luascript examples/31_ui_module.lsc
```

**Prerequisites for the tagged build:** cgo enabled (`CGO_ENABLED=1`, the default when a C compiler is present) and a working C toolchain plus OpenGL development headers:

- **Windows** — a MinGW-w64 GCC, e.g. via [MSYS2](https://www.msys2.org): `pacman -S mingw-w64-x86_64-gcc mingw-w64-x86_64-headers mingw-w64-x86_64-crt`, then ensure `…\mingw64\bin` is on `PATH`.
- **macOS** — Xcode command-line tools (`xcode-select --install`).
- **Linux** — a C compiler plus the GL/X11 dev packages (Debian/Ubuntu: `sudo apt install gcc libgl1-mesa-dev xorg-dev`).

Without the tag the headless stub is used, so the rest of the language builds and runs regardless of whether a C toolchain is installed.

## Bonsai mode

For a break from the language work, `luascript` ships with a small ASCII-bonsai grower. It is unrelated to the Lua runtime — just a fun side mode.

```sh
# Grow a tree in the alt-screen (press q or Ctrl+C to leave)
./luascript -bonsai

# Print a single tree to stdout instead
./luascript -bonsai -bonsai-print

# Animate growth step-by-step
./luascript -bonsai -bonsai-live

# Reproducible tree from a seed
./luascript -bonsai -seed 42

# Attach a message next to the tree
./luascript -bonsai -bonsai-msg "hello, world"
```

| Flag            | Effect                                                         |
| --------------- | -------------------------------------------------------------- |
| `-bonsai`       | Grow an ASCII bonsai tree and exit.                            |
| `-seed N`       | RNG seed for reproducible trees (`0` = random).                |
| `-bonsai-print` | Print the tree to stdout instead of staying in the alt-screen. |
| `-bonsai-live`  | Animate growth step-by-step.                                   |
| `-bonsai-msg S` | Attach a message next to the tree.                             |

## Examples

A walk-through set lives in `examples/`. Most are runnable straight from the
repo root with `go run ./cmd examples/<file>`:

| File                           | What it shows                                                                                                                         |
| ------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------- |
| `01_basics.lsc`                | variables, control flow, primitive values, logical operators                                                                          |
| `02_functions.lsc`             | recursion, closures and upvalues, multi-return, higher-order functions                                                                |
| `03_tables_and_metatables.lsc` | records, arrays, methods, operator overloading via `__add`, `__index`                                                                 |
| `04_coroutines.lsc`            | `coroutine.create` / `resume` / `yield` / `wrap`                                                                                      |
| `05_types.lsc`                 | the full Luau-style type surface — primitives, optionals, unions, function types, type aliases, type assertions                       |
| `06_strict_mode.lsc`           | `--!strict` enforcement and what it rejects                                                                                           |
| `07_modules.lsc`               | `require`, `package.path`, `package.loaded`, `searchpath` — imports `mathx.lsc` next to it                                            |
| `08_stdlib.lsc`                | a bundled-library set loaded via `LUASCRIPT_LIB` — flat modules, dotted submodules, package `init` files                              |
| `09_native_module.lsc`         | importing a host-provided native module (`native/db`)                                                                                 |
| `10_os_module.lsc`             | importing a host-provided native module (`native/os`)                                                                                 |
| `11_compounds.lsc`             | compound assignment operators (`x op= e`)                                                                                             |
| `12_math_module.lsc`           | the `math` native module                                                                                                              |
| `13_json_module.lsc`           | the `json` native module                                                                                                              |
| `14_match.lsc`                 | `match` statement — parser-level desugar into `if/elseif`, `_` wildcard                                                               |
| `15_http_module.lsc`           | the `http` client native module (shortcuts, `http.request{...}`, stateful clients)                                                    |
| `16_httpserver_module.lsc`     | the `httpserver` native module — handlers, `:listen` / `:stop`                                                                        |
| `17_crypto_module.lsc`         | the `crypto` native module — hashing, HMAC, random bytes                                                                              |
| `18_time_module.lsc`           | the `time` native module — durations, timers, formatting                                                                              |
| `19_regexp_module.lsc`         | the `regexp` native module (Go regex; `:capture`, not `:match`)                                                                       |
| `20_uuid_module.lsc`           | the `uuid` native module                                                                                                              |
| `21_sort_module.lsc`           | the `sort` native module — `sort.sort` / `stable` / `reverse` / `is_sorted`                                                           |
| `22_string_interpolation.lsc`  | backtick string interpolation: `` `hello {name}` `` desugars to `..`-concat                                                           |
| `23_io.lsc`                    | the full Lua-5.4 `io` library (file handles, `:read`/`:write`/`:lines`/`:seek`)                                                       |
| `24_bit_utf8.lsc`              | the `bit32` and `utf8` native modules                                                                                                 |
| `25_os_full.lsc`               | the expanded `os` parity surface (`date`, `time`, `clock`, `execute`, `rename`, `tmpname`, `setlocale`)                               |
| `26_patterns.lsc`              | full Lua-pattern surface (`find`/`match`/`gmatch`/`gsub` with `%a %d %w` classes, `()` captures, `%b()` balanced, `%f[set]` frontier) |
| `27_debug_module.lsc`          | the `debug` native module — `traceback`, `getinfo`, hook stubs                                                                        |
| `28_compression_module.lsc`    | the `compression` native module — gzip, zlib, deflate, run-length (`rle_encode`/`rle_decode`)                                          |
| `29_enums.lsc`                 | `enum Name V1, V2 end` — int-auto-increment, frozen via `__newindex` proxy                                                            |
| `30_std_module.lsc`            | the `std` native module — stack, queue, deque, set, list, heap (requires `cmp`), hashmap                                              |
| `31_ui_module.lsc`             | the `ui` desktop module (Fyne) — windows, widgets, layouts. Run with `-tags luascript_ui` (see [Desktop UI module](#desktop-ui-module-opt-in)) |
| `32_defer.lsc`                 | `defer call()` — LIFO cleanup that runs on normal return, fall-off-end, and error unwinding                                            |
| `33_typeof_sizeof.lsc`         | the `typeof` / `sizeof` reflection builtins — int/float distinction, `__type` metatable hook, byte/entry sizes                        |
| `34_clustering_module.lsc`     | the `clustering` native module — k-means (k-means++ seeding), DBSCAN, hierarchical/agglomerative, mean-shift                          |
| `35_classification_module.lsc` | the `classification` native module — Naive Bayes (text), KNN, perceptron, logistic regression, SVM (linear + RBF kernels)            |
| `36_math.lsc`                  | the `math` native module in depth — Lua 5.4 scalar surface plus statistics helpers (`mean`/`variance`/`standard_deviation`/`softmax`) |
| `37_stats_module.lsc`          | the `stats` native module — descriptive/inferential statistics (median, mode, quantiles, iqr, covariance, correlation, skew/kurtosis, zscore/normalize, describe) |
| `38_linalg_module.lsc`         | the `linalg` native module — vectors and matrices (dot, norm, matmul, transpose, det, inverse, solve)                                  |
| `39_csv_module.lsc`            | the `csv` native module — parse/stringify/read/write with header + numeric coercion and custom delimiters                             |
| `40_dataframe_module.lsc`      | the `dataframe` native module — column-oriented tables (select/filter/with_column/sort/group_by/describe/to_csv, pretty `print`)       |

### Running the module examples

`require` resolves a module name against `package.path`. The two entry kinds
that matter for these examples, searched in this order:

1. **The directory of the script being run** — added automatically. So a
   module sitting next to your script is always found, no matter which
   directory you launched from. This is why `07_modules.lsc` just works:

   ```sh
   go run ./cmd examples/07_modules.lsc     # mathx.lsc is found next to it
   ```

2. **`LUASCRIPT_LIB`** — a bundled-library root, read once at startup. It
   is _not_ on the path unless you set it. `08_stdlib.lsc` is the demo for
   exactly this: its modules live under `examples/stdlib/` (not next to the
   script). For convenience the example self-bootstraps `package.path` so
   `go run ./cmd examples/08_stdlib.lsc` works without setting the env
   var, but the canonical invocation is still:

   ```sh
   # bash
   LUASCRIPT_LIB=./examples/stdlib go run ./cmd examples/08_stdlib.lsc
   # PowerShell
   $env:LUASCRIPT_LIB="./examples/stdlib"; go run ./cmd examples/08_stdlib.lsc
   # cmd.exe
   set LUASCRIPT_LIB=./examples/stdlib && go run ./cmd examples/08_stdlib.lsc
   ```

   `LUASCRIPT_LIB` is resolved relative to your current working directory —
   if you run from somewhere other than the repo root, adjust the path
   accordingly (e.g. `../examples/stdlib` from inside `cmd/`).

Between the two, the plain cwd-relative entries (`./?.lsc`, `./src/?.lsc`,
...) are searched as well, so a module under your working directory is still
found even when it sits nowhere near the script.

The native-module examples pull their modules from the host via
`package.preload`, so they need neither a path entry nor `LUASCRIPT_LIB`.

A taste, in case you don't want to open files:

```lua
-- factorial
local function fact(n)
    if n <= 1 then return 1 end
    return n * fact(n - 1)
end
print(fact(10))   -- 3628800

-- closures + upvalues
local function counter()
    local n = 0
    return function()
        n = n + 1
        return n
    end
end
local next = counter()
print(next(), next(), next())   -- 1   2   3

-- coroutines
local co = coroutine.create(function()
    for i = 1, 3 do coroutine.yield(i) end
end)
print(coroutine.resume(co))   -- true 1
print(coroutine.resume(co))   -- true 2

-- types
type Point = { x: number, y: number }

local function dist(p: Point): number
    return math.sqrt(p.x * p.x + p.y * p.y)
end

print(dist({ x = 3, y = 4 }))   -- 5.0
```

## Type system

LuaScript's type system is **gradual** in the Luau sense: annotations are optional, untyped code is treated as `any`, and `any` flows into and out of any typed slot.

```lua
-- Annotations on locals, parameters, returns. Untyped slots stay any.
local count: number = 42
local name: string = "Ada"
local maybe: string? = nil           -- T?  ≡  T | nil
local id: number | string = "user-7" -- unions

-- Function types — params, returns, multi-return, varargs.
local function add(a: number, b: number): number
    return a + b
end

local function pair(x: number): (number, number)
    return x, x * 2
end

-- Type aliases — including structural table shapes.
type Point = { x: number, y: number }
type Callback = (number) -> string
type Numbers = { number }            -- array shorthand for {[number]: number}

local origin: Point = { x = 0, y = 0 }

-- Type assertions: programmer-controlled cast. Runtime is a no-op.
local raw: any = 7
local n: number = raw :: number
```

### Mode directives

A leading `--!strict`, `--!nonstrict`, or `--!nocheck` on the first line of a file controls how strictly that file is checked.

| Directive      | Effect                                        |
| -------------- | --------------------------------------------- |
| (none)         | Default. Gradual checking.                    |
| `--!strict`    | Implicit-any parameters become errors.        |
| `--!nonstrict` | Same as the default. Useful for explicitness. |
| `--!nocheck`   | Skip the type pass for this file entirely.    |

### Not in v1 (deliberately)

- Generics (`function f<T>(x: T): T`)
- Intersection types (`A & B`)
- Type refinements (narrowing inside `if type(x) == "string"`)
- String-singleton types (`"foo" | "bar"`)
- Cross-module type checking — `require()` returns `any`
- Recursive type aliases (the parser accepts them; the resolver doesn't)

These are explicitly named in error messages where relevant, so users hit a clear wall instead of silent miscompiles.

## REPL

Launch with `go run ./cmd` (no arguments). Built-in commands:

| Command        | Effect                                             |
| -------------- | -------------------------------------------------- |
| `help`         | print the help screen                              |
| `exit`, `quit` | leave the REPL                                     |
| `reset`        | rebuild the VM (clears all globals and user state) |
| `clear`        | clear the screen                                   |

Key bindings: **Ctrl+C** cancels the current input, **Ctrl+D** exits, **Ctrl+R** searches history.

Bare expressions print their value:

```
luascript » 1 + 2
=> 3
luascript » {1, 2, 3}
=> table: 0xc000...
```

Top-level `local` persists across REPL chunks (it's promoted to a global at compile time so subsequent inputs can read it):

```
luascript » local greeting = "hi"
luascript » print(greeting)
hi
```

Inside any nested scope (`do`/`if`/`for`/function body) `local` keeps standard Lua semantics.

Incomplete input opens a continuation prompt:

```
luascript » function double(x)
   ...      return x * 2
   ...    end
luascript » print(double(21))
42
```

Type errors land with a distinct prefix so they're easy to spot:

```
luascript » local x: number = "hi"
type-error: Type "string" could not be converted into "number" at line 1
```

## Project layout

```
.
├── compiler/
│   ├── lexer/         token stream from source text
│   ├── token/         token types and keyword table
│   ├── parser/        recursive-descent parser, Pratt-style for expressions
│   ├── ast/           AST node definitions (statements, expressions, types)
│   ├── typecheck/     gradual type system — Type representation, env, pass
│   ├── optimize/      AST constant-folding pass (Lua-5.4-safe subset)
│   ├── analyze/       pass-registry static analyzer (`luascript analyze`)
│   ├── debug/         pprof Start/Stop wrappers used by `luascript profile`
│   ├── bytecode/      AST → instruction-set generator
│   └── compiler.go    top-level pipeline (lex → parse → typecheck → optimize → bytecode)
├── vm/                stack VM, closures, metatables, coroutines, stdlib
├── native/            bundled native modules (db, os, http, std, log, …)
├── formatter/         `luascript fmt` — trivia-preserving formatter
├── bonsai/            ASCII bonsai tree side mode (cbonsai/gobonsai fork)
├── repl/              interactive REPL (readline + engine wrapper)
├── examples/          runnable .lsc programs that double as tutorials
├── scripts/           helper scripts (`build-pgo.sh`)
├── version/           version string
└── cmd/               CLI entrypoint (main.go) + `luascript build` bundler
```

The compiler is designed so each stage is independently testable and the AST is the only contract between parser, type checker, and bytecode generator. The VM never sees source text or types; the parser never sees instructions.

## Non-goals (for now)

- Garbage-collection metamethods (`__gc`, `__close` enforcement).
- Generics, intersections, refinements, string-singleton types, cross-module type checking, recursive aliases (type-system v1 deliberately omits these; see "Not in v1" above).

These are deliberate omissions, not bugs — they're listed so contributors know what's out of scope rather than wondering whether to file an issue.

Previously listed but now shipped: Lua patterns, `io.open` + full file-handle stdlib, expanded `os`, the `debug` library.

## Contributing

Run the full test suite before sending a change:

```sh
go test ./...
```

Tests live next to the code they cover (`*_test.go`). The bytecode tests in particular are useful: they assert exact opcode sequences for representative source snippets, which catches accidental codegen drift early. The type checker has its own focused suite under `compiler/typecheck/checker_test.go`.

## Inspirations

- **Lua 5.4** — the syntax and semantics target.
- **Luau** — the type-system shape.
- **Goby** — the original stack VM and bytecode-generator scaffolding (this project is a Goby fork in spirit, though much has been rewritten).

## License

See `LICENSE`.
