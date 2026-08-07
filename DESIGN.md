# LuaScript — design notes

How the implementation is put together and, where it matters, why. This is the
document to read before changing the compiler or the VM. For usage, see
[README.md](README.md); for contribution mechanics, see
[CONTRIBUTING.md](CONTRIBUTING.md).

## Contents

- [Goals and non-goals](#goals-and-non-goals)
- [The pipeline](#the-pipeline)
- [Compiler stages](#compiler-stages)
- [What is desugared, and where](#what-is-desugared-and-where)
- [The type system](#the-type-system)
- [The VM](#the-vm)
- [try / catch](#try--catch)
- [Concurrency: the one rule](#concurrency-the-one-rule)
- [Native modules](#native-modules)
- [Module resolution](#module-resolution)
- [Bytecode serialization and the compile cache](#bytecode-serialization-and-the-compile-cache)
- [Bundled executables](#bundled-executables)
- [Documentation as data](#documentation-as-data)
- [Tooling around the core](#tooling-around-the-core)
- [Repository layout](#repository-layout)
- [Testing and verification](#testing-and-verification)

## Goals and non-goals

LuaScript tracks **Lua 5.4** semantics and adds **Luau-style gradual types**.
Three constraints shape almost every decision below:

1. **Readable end-to-end.** A clean-room lex → parse → check → optimize →
   bytecode → stack-VM pipeline. No LLVM, no JIT, no generated parser.
2. **Types are compile-time only.** They are erased before bytecode generation.
   The VM has no notion of a type annotation, so the runtime is exactly the
   runtime you would have without the type system.
3. **Each stage is independently testable,** and the AST is the only contract
   between parser, checker, and generator. The VM never sees source text or
   types; the parser never sees instructions.

Non-goals are listed inline in the sections they belong to, and collected under
[Deliberately out](#deliberately-out).

## The pipeline

`internal/compiler/compiler.go::CompileToInstructionsWith` is the single
contract between stages:

```
source ──► lexer ──► parser ──► constcheck ──► typecheck ──► optimize.Fold ──► bytecode.Generator ──► VM
                                    │             │
                                    │             └─ gated by --!strict / --!nonstrict / --!nocheck
                                    └─ always on (NOT disabled by --!nocheck)
```

`constcheck` running unconditionally is deliberate: `local x <const>` is a
scoping guarantee, not a type-checking nicety, so `--!nocheck` must not be able
to switch it off.

## Compiler stages

| Package | Responsibility | Notes |
| ------- | -------------- | ----- |
| `lexer/` | Lua 5.4 tokens, long-bracket strings/comments, mode directives | Stamps `Token.Column` for error reporting |
| `token/` | Token kinds + keyword table | |
| `parser/` | Recursive-descent, Pratt for expressions | Parses Lua 5.4 *and* Luau type syntax |
| `constcheck/` | Rejects assignment to `<const>` / `<close>` locals | Always on; scope-tracking |
| `typecheck/` | Gradual type system | Erased after this point |
| `optimize/` | AST constant folding | Lua-5.4-safe subset |
| `bytecode/` | AST → instruction set | Typed `A`/`B`/`StrA`/`BoxedAny` fields |

### Parser specifics worth knowing

- `parser.New()` returns an **un-primed** parser. Sub-parsers built outside
  `ParseProgram` must call `sub.nextToken()` twice before parsing, or
  `parseExpression` silently returns nil.
- Errors use a construct-aware format —
  `<construct>: <expected> got <found> at line N, column C` — plus an indented
  `hint:`, all funnelled through `errorAt(tok, category, construct, msg, hint)`.
- `Parser.loopDepth` gates both `break` and `continue`. Function bodies save and
  zero it, so neither escapes a function boundary (matching Lua/Luau).
- **Hard keywords:** `match`, `enum`, `defer`, `try`, `catch`, `throw`.
  **Contextual keywords:** `type`, `struct`, `continue`. A contextual keyword is
  recognized only when the next token can't extend it into an expression
  (`peekStartsSuffix()`), so `continue = 1` and `continue()` still parse as
  identifiers.
- `match` is hard, but permitted as a field/method name after `.` / `:` via
  `curTokenIsFieldName()` — otherwise `string.match` would not parse.
- `catch` is a **block terminator** (alongside `end`/`else`/`until`). That is
  what lets a `try` body end without a `do`/`end` of its own, with a single
  `end` closing the whole statement. The error binding is optional but the `do`
  is not: a handler body can itself start with an identifier, so `catch <Name>
  do` vs `catch do` is the only unambiguous split.
- Match destructure patterns are gated on names declared in the chunk
  (`Parser.structNames` / `Parser.enumVariants`). `Circle(r)` destructures
  positionally only when `Circle` is a payload-carrying tagged-enum variant, and
  `Point{ x = a }` only when `Point` is a struct. Any other call-shaped pattern
  is a value pattern (call + compare).

### AST note

The live match representation is the `Kind`-tagged `ast.MatchPattern` /
`ast.MatchStmtArm` / `ast.MatchStatement` in `ast/statements.go`. Every consumer
(parser, typecheck, constcheck, optimize, bytecode, formatter, analyze) uses it.

## What is desugared, and where

A recurring question is whether a construct is "real". This table is the answer:

| Construct | Representation | Where it is lowered |
| --------- | -------------- | ------------------- |
| Compound assign (`+= -= *= /= \|= &= <<= >>=`) | none | parser desugar |
| Backtick string interpolation | none | parser desugar (to `..`) |
| `match` statement | real AST node | bytecode generator |
| `enum` | real AST node | bytecode generator → `__enum_freeze` |
| `continue` | real AST node | jumps to the loop's `continueAnchor` |
| If expressions | real AST node | jump chain; folded when the condition is literal |
| Default parameters | `TypedParam.Default` | codegen prologue |
| `defer` | real | frame-local closure list |
| `try` / `catch` / `throw` | real AST nodes | dedicated opcodes (see below) |
| Type assertions (`x :: T`) | real AST node | erased — runtime no-op |

Notes on the non-obvious ones:

- **`enum`** lowers to `local Name = __enum_freeze({V1=1, V2=2, ...}, "Name")`.
  The `__enum_freeze` global comes from `internal/native/stdlib/enumrt` — an
  int-auto-increment table frozen behind a `__newindex` proxy. `enumrt` sits in
  `nativeRegistrars` purely so the helper reaches both VM code paths; it is not
  a `require` target. Typecheck treats a bare enum alias as `number`.
- **`continue`** jumps to the anchor right after the body, before the
  per-iteration `CloseUpvalues` and the condition/step re-check. `break` closes
  upvalues too — every loop form re-emits `CloseUpvalues` on its exit path. In
  `repeat`, `continue` jumps to the `until` condition, so an `until` that reads
  a local declared after a `continue` is a **compile error** (as in Luau),
  enforced by `checkRepeatContinueLocals`.
- **Default parameters** compile to the equivalent of
  `if x == nil then x = expr end`, so `false` does **not** trigger the default,
  and a default can see earlier parameters. The checker widens a defaulted
  parameter with `nil` in the *signature* (callers may omit it) while binding the
  declared type inside the body.
- **`defer`** captures by upvalue, so a deferred call observes a variable's value
  at exit time — unlike Go, which snapshots arguments eagerly.

### REPL local promotion

`bytecode/statement_generation.go::isReplTopLevel()` rewrites chunk-root
`local x = v` (and `local function f`) to `SetGlobal` when running in
`parser.REPLMode`, so bindings survive across REPL chunks. Locals in any nested
scope are untouched and keep standard Lua semantics. This is why the bundled-exe
path must use `parser.NormalMode` — `REPLMode` would leak a script's top-level
locals into globals.

## The type system

Gradual in the Luau sense: annotations are optional, unannotated slots are
`any`, and `any` flows into and out of any typed slot. Stdlib signatures are
hand-written in `typecheck/stdlib_types.go`. Table literals are inferred as
`any` so dynamic Lua patterns keep working.

**In:** primitives, function types (params/returns/multi-return/varargs),
optionals (`T?`), unions (`A | B`), type aliases including structural tables,
type assertions, structs, tagged enums.

**Also in (post-v1):**

- **Generics** (`typecheck/generics.go`) — generic aliases and structs
  (`type Box<T>`, `struct Pair<A, B>`), instantiated on application, plus
  best-effort call-site inference for generic functions. Inside a generic body a
  type variable is opaque but gradual, so parametric code never produces
  spurious errors.
- **Refinements / narrowing** (`typecheck/refine.go`) — type guards
  (`type(x) == "T"`), nil guards, truthiness, `not`/`and`/`or` propagation,
  elseif negation accumulation, `assert(cond)` narrowing for the rest of the
  block, and short-circuit RHS narrowing (`s ~= nil and #s`; `x or default` also
  drops `nil`).
  - **Early-exit narrowing:** a leading prefix of always-terminating if-clauses
    (return/break/continue/throw/`error()`) persists its negations past the
    `end`. `goto` is deliberately not treated as a terminator.
  - Only simple identifiers refine — field paths (`x.y`) are not tracked.
  - Narrowing shadows are marked in the env (`defineRefined`); assignments check
    against the *declared* type (`lookupDeclared`) and widen active shadows
    (`widenRefined`), so a stale refinement can never vouch for a dead value.

### Mode directives

A leading `--!strict`, `--!nonstrict`, or `--!nocheck` on line 1 sets the file's
strictness. `--!strict` turns implicit-any parameters into errors; `--!nocheck`
skips the type pass entirely (but **not** `constcheck`).

### Deliberately out

Intersections (`A & B`), string-singleton types (`"foo" | "bar"`), cross-module
type checking (`require()` returns `any`), and recursive aliases (the parser
accepts them; the resolver does not). These are named explicitly in error
messages so users hit a clear wall rather than a silent miscompile.

Also out at the language level: GC metamethods (`__gc`, and `__close`
enforcement — the attribute is parse- and const-checked only); full `debug`
semantics (`debugx` ships real `traceback`/`getinfo` but hook **stubs**, not
VM hooks); `finally` on try/catch and type-filtered `catch` clauses (`defer`
already covers unconditional cleanup, and a handler can re-`throw` after
inspecting the value — the catch binding is always `any` because the checker
cannot narrow what a `throw` produces).

## The VM

Stack-based, in `internal/vm/`. Closures, metatables, coroutines (goroutines +
channels), and `pcall`/`error` unwinding. Performance-sensitive pieces: a
`framePool` capped at 256, `pushNils`, and `bsearch`-based open-upvalue lookup.

**The FFI rule.** Only `nil`, `bool`, `int64`, `float64`, `string`, `*Table`,
`*Closure` and `*GoFunc` are runtime-tracked. Cast raw Go `int`, `FileMode`,
`rune`, etc. to `int64` at the boundary. Argument helpers live in
`vm/stdlib_args.go` (`NumArg`/`IntArg`/`FloatArg`/`StringArg`, `TableArg`,
`ClosureArg`, `CoroutineArg`, `AnyArg`, `NilOrTableArg`, `TableOrStringArg`,
`OptString`, `OptInt`) — prefer these over hand-rolled panics.

**`__tostring` is honoured** by `tostring`, `print`, `io.write`,
`error(value)`, and the REPL's value printer, all routed through
`vm.ToStringMM(v, val)` in `vm/value.go`. Numbers, strings, bools and nil skip
the metamethod lookup; tables and userdata route through `__tostring` when
present and panic if it returns a non-string.

**Multi-return spread into a call** (e.g. `print("x", string.gsub(...))`) works
via the `MarkArgs` opcode. When `compileCall` / `compileMethodCall` sees a
multi-value last argument it emits `MarkArgs` to record the stack height; the
matching `Call` is emitted with `nargs=-1` and `doCall` recovers the args base
from `v.callMarks`. Static, fixed-arity calls keep the fast path and pay no mark
overhead.

**Arithmetic / comparison fast paths.** `Add`, `Sub`, `Mul`, `Lt` and `Le` have
inline int+int and float+float paths in the dispatch loop that skip the
string-keyed `arithMM`/`lessMM` dispatch. Mixed types and metatable paths still
go through the full `*MM` helpers.

**Lua patterns.** `string.match`, `gmatch`, `gsub` and `find` implement the full
Lua pattern surface — classes (`. %a %d %s %w` and complements), `[set]` with
ranges, `^ $` anchors, `* + - ?` quantifiers, `( )` captures including empty
position captures, `%1..%9` backrefs in both patterns and replacements, `%b()`
balanced, and `%f[set]` frontier. `string.find` engages the engine only when
magic characters are present, keeping a plain-substring fast path otherwise. The
engine is `vm/patterns.go`.

### Error positions and tracebacks

`vm/traceback.go` is the whole of the error-reporting surface. Two facts drive
its shape.

**Frames survive the panic.** A raised error unwinds the *Go* stack, but
`v.frames` is VM state that nothing touches on the way out — `execCatching`
re-panics without unwinding when it has no handler for the error. So at the
moment an error is finally caught, the Lua call stack that produced it is still
intact and can be read. Exactly four places catch one and then destroy those
frames:

| boundary | catches for |
| -------- | ----------- |
| `safeCall` | `pcall` / `xpcall` / `VM.SafeCall` |
| `dispatchToHandler` | a `try` region |
| `Coroutine.goroutineBody` | a coroutine dying, reported through `resume` |
| `recoverToError` | the error reaching the host uncaught |

Each calls `v.errorValue(r)` (or `v.toRuntimeError(r)` at the top) as the first
statement in its recover, before any unwinding. Everywhere else the panic is
re-panicked untouched, so the *deepest* boundary is always the one that records
the position — which is the raise site, not the handler.

**The panic's type says who owns the position.** `LuaError` (and a bare Go
`error`) means the VM raised — "attempt to index a nil value" — and gets the
`<source>:<line>: ` prefix stamped on. `luaError` means a script raised via
`error`/`assert`/`throw`, where positioning is the raiser's business: `error`
already applied it at the requested level, and `throw` is deliberately verbatim.
That split is what keeps a value from being prefixed twice.

An uncaught error reaches the host as a **`*RuntimeError`** carrying the raised
value, the positioned message, and the captured stack. Its `Error()` renders
message plus traceback, which is what the CLI prints; `Message()` is the bare
message. A stack that is only the main chunk renders without a traceback — it
would repeat what the message's own prefix already said.

**Chunk names are stamped at load, not compile.** `InstructionSet.SetSource`
walks a chunk and its nested protos, and is called by whoever read the chunk:
`repl.RunFile`, `require`, `loadfile`, `load` (honouring its chunkname argument
and Lua's `=`/`@` sigils), and `luascript profile`. It deliberately does not run
in the generator and is not serialized, because the bytecode cache is keyed on
*content*: one cached chunk may legitimately be loaded from two paths. A chunk
nobody stamped reports as `script`.

Captures are bounded: `Traceback` keeps the innermost 10 and outermost 11
frames and collapses the rest into a `... (skipping N levels)` marker, so a
runaway recursion against a 200,000-frame ceiling does not render — or even
allocate — one line per frame.

Function names in a traceback come from the proto name, which the generator now
takes from the binding when there is one: `local function f`, `function a.b:c`,
`local f = function() end` and `M.run = function() end` all name their literal.
A genuinely anonymous literal falls back to `anon@<line>`, which at least
locates its definition. `debug.traceback` and `debug.getinfo` render through the
same `vm.Traceback` / `vm.FormatTraceback` pair, so the module and an uncaught
error cannot drift apart.

Known gap: `xpcall` runs its message handler *after* unwinding, so a handler
that calls `debug.traceback` sees its own stack rather than the failed call's.
Lua runs the handler before unwinding; matching that means calling back into Lua
from inside the recover, which is a larger change than this bought.

## try / catch

`try` is a **real protected region in the enclosing frame** — deliberately *not*
a `pcall`-plus-closure lowering. That choice is load-bearing: it is what makes
`return`, `break` and `continue` inside a `try` act on the enclosing function or
loop, and what keeps the body's locals ordinary frame slots.

- **Opcodes.** `Try` (A = catch IP) pushes a handler; `EndTry` (A = count) pops
  N; `Throw` pops a value and raises it. `throw` being an opcode rather than a
  call to `error` means shadowing `error` cannot re-point it. (`error` itself
  propagates its argument verbatim, so the two are otherwise identical.)
- **Handlers live on the `CallFrame`** (`f.handlers`), not on the VM. Two
  consequences fall out: a `return` out of a `try` needs no bookkeeping, since
  unwinding the frame discards the handlers; and a coroutine's handlers travel
  with its frames across the yield/resume state swap.
- **The recover point is `execCatching`,** entered from `exec` only when the
  proto's `HasTry()` is set — so every other call pays one predictable branch
  and nothing more. `HasTry` is derived by `scanProto`, the same one-time cached
  scan that resolves `NumLocals`, which is why a proto loaded from the bytecode
  cache gets it for free with no extra serialized field.
- **`dispatchToHandler`** performs the same restoration `safeCall` does —
  abandoned frames' defers, then `closeUpvaluesAbove`, then truncate
  frames/stack/**callMarks** — but stops at the try's frame. An unhandled error
  is **re-panicked**, so it keeps unwinding to an outer try, a `pcall`, or the
  host.
- **Every non-raising exit emits `EndTry`:** the body's fall-through, and any
  `break`/`continue` that escapes one (`Generator.exitTryDepth`, stamped onto
  `loopFrame.tryDepth` by `pushLoop`). **Every loop form must push through
  `pushLoop`** — otherwise a `break` inside a `try` leaves a stale handler and a
  later error lands in a catch that has already been jumped out of.
- **`goto` across a `try` boundary** (either direction) is a compile error, not
  a miscompile — see `checkGotoTryDepth`.

## Concurrency: the one rule

**Lua code runs only on the VM goroutine, one call at a time.**

This is not a policy choice that can be relaxed. The VM has no locks: `Globals`
is a plain map, open upvalues point into `&vm.Stack`, and the `GetGlobal` inline
cache writes back into the shared `*bytecode.Instruction`. Running Lua on two
goroutines is a torn read, not a speedup.

The `queue` module (`internal/native/stdlib/queue/`) is built around that
invariant by splitting scheduling from execution:

- `dispatcher.go` is a thread-safe **scheduler** — two heaps (ready, ordered by
  priority then a monotonic `seq` for FIFO; and delayed, ordered by `ReadyAt`).
  It **runs nothing**. Submitting is safe from any goroutine.
- `queue.go::pump` drains the dispatcher on the VM goroutine and is the **only**
  place a job is invoked, always via `vm.SafeCall` (never `CallValue`) so a
  failing job cannot leave the shared VM dirty. `q:run()` blocks and drains;
  `q:poll()` takes only what is due.
- Goroutines exist only in `queue.after` and `queue.tick`, which push into a
  channel and never touch the VM.

Do **not** "improve" this with a worker pool that calls Lua — that was the
original bug. Two corollaries follow: `timeout_ms` is a deadline on *starting*
(expired jobs are shed unrun; a running Lua call cannot be preempted), and
`Channel` never closes its data channel — `Close` closes a separate `done`
channel, so a send racing a close reports `Closed` instead of panicking.

`httpserver` follows the same shape: `:listen` blocks on the VM goroutine and
dispatches handlers through a buffered `jobCh` (cap 64), with an 8 MiB body cap
(→413), `:stop()`, and a clean `ErrServerClosed` exit.

## Native modules

**Single source of truth:** `cmd/luascript/natives.go::nativeRegistrars` is the
only list of bundled native modules. Both code paths walk it:

- `cmd/luascript/main.go` → `repl.AddPostInit` (CLI path; re-applied on REPL
  `:reset`).
- `cmd/luascript/build.go::runBundled` → `registerAllNatives` (bundled binary).

Each native package exports a one-line `RegisterX(v *vm.VM)` that calls
`vm.RegisterPreload(v, name, loader)` (helper in `vm/preload.go`). **Adding a
native module is one line** in `nativeRegistrars`. Native method names must
avoid Lua reserved words — which is why `regexp` exposes `:capture`, not
`:match`.

Native modules arrive via `package.preload` on the host side, not the path
search.

### Platform- and toolchain-gated modules

Two modules ship a real backend and a stub behind build constraints, so
`require` resolves everywhere and scripts can branch at runtime:

- **`ui`** (Fyne desktop GUI) is **off by default** because Fyne pulls in OpenGL
  via cgo. The default build compiles `ui_stub.go`: `require("ui")` resolves but
  errors on first widget construction. `-tags luascript_ui` selects
  `ui_fyne.go` and needs CGO plus a C toolchain and OpenGL headers.
- **`plugin`** (load Go packages at run time) needs cgo *and* a platform where
  Go supports plugins. `backend_native.go` is `//go:build (linux || darwin ||
  freebsd) && cgo`; `backend_stub.go` covers everything else. **Windows can
  never run plugins** — `plugin.supported` is `false` there and
  `generate`/`open` raise.

There is no postgres build tag: `lib/pq` is a plain blank import in
`internal/native/stdlib/db/db.go`. The intended pattern for further drivers is a
`driver_<name>.go` file with a `//go:build` directive.

### The `plugin` module in more detail

A script declares packages and functions; the module renders a `package main`
re-exporting them as package-level vars, compiles it with
`go build -buildmode=plugin`, opens the `.so`, and dispatches through
`reflect`.

- **Lua→Go conversion is driven by the target parameter type** (`fn.Type().In(i)`),
  so one Lua integer satisfies a Go `int`, `float64` or `time.Duration`. Go→Lua
  obeys the FFI rule (ints widen to `int64`, floats to `float64`, `[]byte` →
  string, slices/maps → `*Table`). A returned `error` keeps its position and
  becomes `nil` or its message string.
- Values with no Lua counterpart (structs, pointers, interfaces — e.g.
  `*sql.DB`) are wrapped as a **GoValue** using the same `*Table` +
  private-key + shared-metatable pattern as `ndarray` (key `"\x00govalue"`),
  whose `__index` resolves exported methods and fields by reflection, so
  `db:Query(...)` works. Passing a GoValue back into Go unwraps it.
- A generated `var X = pkg.X` comes back from `plugin.Lookup` as a **pointer** to
  the var, so `loadedPlugin.lookup` dereferences pointer-to-func before calling.
  Non-func pointers are left alone, keeping pointer-receiver methods reachable.
- Artifacts live under
  `os.UserCacheDir()/luascript/plugins/<name>-<sha256(source)[:16]>[-race]/`,
  one directory per plugin (two `func main`s cannot share a package).
  `LUASCRIPT_PLUGIN_DIR` relocates it. An unchanged spec is a cache hit; only
  specs with non-stdlib imports run `go mod tidy`.
- **A plugin must match the host's build config, `-race` included.**
  `raceEnabled` (build-tagged, `race.go` / `race_enabled.go`) forwards `-race` to
  the plugin build and appends `-race` to the cache directory. Without the
  suffix a race-enabled host would get a cache *hit* on a non-race `.so` and
  `plugin.Open` would reject it. Same class of constraint as the Go version
  pinned in `goMod`.
- The plugin imports **only** the requested packages — never `internal/vm` — so
  "plugin was built with a different version of package X" is limited to a
  toolchain or `-race` mismatch. A plugin importing a package the host also
  links (e.g. `lib/pq`) can still hit it if versions drift.
- `generate` runs the Go compiler and loads native code: **arbitrary code
  execution by design.** Specs are validated (identifiers, import paths) to turn
  typos into Lua errors, not as a security boundary.

### Data-science modules

`ndarray`, `dataframe`, `csv`, `stats`, `linalg`, `clustering`,
`classification`, `ml` (package `luaml`) and `plot`. `ndarray` values are
`*Table` wrappers sharing one metatable, carrying the backing `*ndarray` under a
private instance key (`"\x00ndarray"`); 0-D results (vector dot, full
reductions) come back as bare Lua numbers rather than wrapped values. `plot` is
dependency-free SVG charting and uses the same wrapper pattern.

Note that `internal/native/constraints` is a Go generics type-constraint helper
package, not a Lua module.

## Module resolution

`require` resolves against `package.path`, in this order:

1. The directory of the script being run (added automatically).
2. cwd-relative entries (`./?.lsc`, `./src/?.lsc`, …).
3. `$LUASCRIPT_LIB`, only if set (read in `vm/loader.go`).

## Bytecode serialization and the compile cache

`bytecode/serialize.go` provides `SerializeChunk` / `DeserializeChunk` (magic
`LSCB`, `SerialVersion`). It round-trips the main `InstructionSet` plus nested
`Protos`; `Params` are reconstructed from the typed fields on load (the inverse
of `encodeParams`), so the disassembler works on deserialized chunks. The header
embeds `InstructionCount`, so **renumbering opcodes invalidates old chunks even
without a `SerialVersion` bump**.

`internal/compiler/bccache/` is the on-disk compile cache, used by `RunFile`,
`require` and `loadfile` — but **not** by the REPL, `load()` strings, `-dis`, or
bundled executables. Entries live under
`os.UserCacheDir()/luascript/bytecode/<sha256>.lscb`, keyed by source +
interpreter version + serial format + opcode count. They are written atomically
and only after a fully successful compile, so type errors are never cached, and
any decode failure falls back silently to a fresh compile.

Env knobs: `LUASCRIPT_NOCACHE=1` disables it; `LUASCRIPT_CACHE_DIR` relocates it
(the tests use this).

## Bundled executables

`luascript build` appends `source + version + magic("LUASCRIPT01")` to a copy of
the interpreter binary. On startup `cmd/luascript/main.go::readEmbeddedPayload`
inspects its own tail *before* flag parsing; when a trailer is present it runs
the embedded script in `parser.NormalMode` and re-registers all natives via
`registerAllNatives`.

Source-payload only — bytecode serialization, cross-compilation and compression
are v1 non-goals. The bundled binary matches the host platform, bundled scripts
do not see `os.Args`, and antivirus heuristics occasionally flag
self-appending executables (the same trade-off PyInstaller and Bun `--compile`
carry; code-signing resolves it).

## Documentation as data

`internal/docs/` is the single source of truth for the stdlib reference: one
curated `Entry` per global, library function, module member and object method,
grouped into `Topic` man pages (`data_core.go`, `data_library.go`,
`data_modules.go`, `data_datascience.go`, `data_objects.go`) plus a renderer
(`render.go`). It is **data only** — it imports neither the VM nor the compiler,
so every consumer shares it:

- `luascript doc` / `man` (`cmd/luascript/doc.go`) — pages, entries, index,
  apropos (`-k`), `-all`.
- The REPL's `doc <topic>` command (`repl.docCommand` / `printDoc`).
- `internal/lsp/server/builtins.go` derives **all** completion and hover from
  it, keeping no table of its own — so a new entry here reaches the editor too.

**Drift is checked, not assumed.** `luascript doc -audit` (and
`TestDocsMatchRuntime`, the same code path via `auditDocs`) loads every native
module and auto-global into a real VM and diffs the live member set against the
registry, reporting both undocumented and stale names. Topics point at their
runtime surface with `RuntimeModule` / `RuntimeGlobal`; `math` and `io` set both
because they exist twice (a small auto-global and a larger native module) and
are documented as one merged page (`Requireable: true`). Object topics are
exempt — their methods live on constructed values that reflection cannot reach.

## Tooling around the core

| Package | What it is |
| ------- | ---------- |
| `internal/repl/` | Readline-driven REPL, history, continuation prompts; type errors get a `type-error:` prefix |
| `internal/formatter/` | `luascript fmt`; trivia-preserving via `trivia.go` |
| `internal/compiler/analyze/` | Pass-registry static analyzer; read-only, re-parses source, has its own walker |
| `internal/lsp/` | Language server (`luascript lsp`) — `protocol`, `jsonrpc2`, `uri`, `server/` |
| `internal/pkgmanager/` | `luascript pkg` — manifest, lockfile, fetch |
| `internal/compiler/debug/` | pprof Start/Stop wrappers behind `luascript profile` |
| `internal/gctune/` | GC knobs behind `-gc-percent` / `-mem-limit` |
| `internal/bonsai/` | ASCII bonsai side mode (cbonsai/gobonsai fork), unrelated to the runtime |

`internal/lsp/protocol`, `internal/lsp/jsonrpc2` and `internal/lsp/uri` are
adapted from gopls and intentionally keep the **complete** protocol binding,
including the client-side half the server does not call. Reachability tools
report much of it as unused; that is expected and is not dead code to prune —
trimming it would fragment the binding and make re-syncing upstream harder.

`internal/bonsai/` uses tcell v3 (`<-sc.EventQ()` / `Get(x,y)`) and a per-`Run`
RNG so `-seed` is deterministic. Its alt-screen path cannot be verified
programmatically; use `-bonsai-print` for non-interactive runs.

## Repository layout

```
.
├── cmd/luascript/     CLI entrypoint + `luascript build` bundler + natives.go
├── internal/
│   ├── compiler/      lexer, token, parser, ast, constcheck, typecheck,
│   │                  optimize, bytecode, bccache, analyze, debug, compiler.go
│   ├── vm/            stack VM, closures, metatables, coroutines, stdlib
│   ├── native/
│   │   ├── stdlib/    db, os, http, json, std, queue, log, io, …
│   │   └── datascience/  ndarray, dataframe, stats, linalg, ml, plot, …
│   ├── plugin/        run-time Go package loading (cgo, non-Windows)
│   ├── docs/          stdlib reference data + man-page renderer
│   ├── lsp/           language server
│   ├── formatter/     `luascript fmt`
│   ├── repl/          interactive REPL
│   ├── pkgmanager/    manifest / lockfile / fetch
│   ├── bonsai/        ASCII bonsai side mode
│   ├── gctune/        GC tuning helpers
│   └── version/       version string
├── client/            VS Code extension (TypeScript)
├── examples/          runnable .lsc programs that double as tutorials
├── scripts/           build-pgo.sh, benchmark.rb
└── assets/            logo and static assets
```

> **Note:** the repository directory on disk is still named `sakura-lang`
> (pre-rename), and a few Go comments carry sed-rename artifacts (e.g.
> `.lsc profile`, `-tags.lsc_no_postgres` in `native/stdlib/db/db.go`, the
> `scripts/build-pgo.sh` header). The module path, package names, env vars and
> build tags are the authoritative current names.

## Testing and verification

Tests live next to the code (`*_test.go`). `make check` — `gofmt -l .` +
`go vet ./...` + `go test ./...` — is the pre-commit gate.

The **bytecode tests assert exact opcode sequences** for representative source
snippets, so they catch codegen drift early. The type checker has its own suite
in `typecheck/checker_test.go`.

Things automated checks cannot reach, and which therefore need a manual
smoke-test before you rely on them:

- Interactive paths: the REPL, `httpserver`, and anything stdin-driven.
- Alt-screen modes: bonsai (use `-bonsai-print` instead).
- The cgo-gated `ui` build.
- The `plugin` module's real backend, which **cannot be built or run on
  Windows at all**; cross-compiling with `GOOS=linux CGO_ENABLED=1` also fails
  for want of a Linux C toolchain. Verify it in WSL, which runs the actual
  `go build -buildmode=plugin` → `plugin.Open` chain:

  ```sh
  wsl -d Ubuntu-22.04 -e bash -lc 'cd /mnt/d/.../Luascript && CGO_ENABLED=1 go test ./internal/plugin/'
  wsl -d Ubuntu-22.04 -e bash -lc 'cd /mnt/d/.../Luascript && CGO_ENABLED=1 go run ./cmd/luascript examples/53_plugin.lsc'
  ```

  `go test -race ./internal/plugin/` in WSL is worth running too, given the
  `-race` cache-key constraint described above.

Because of the `-race`/build-config coupling and the Windows gap, everything in
`internal/plugin/` except `backend_native.go` is platform-independent and unit
-tested on Windows; `convert.go` in particular imports no `plugin` package so it
compiles and tests everywhere.
