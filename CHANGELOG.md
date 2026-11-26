# Changelog

All notable changes to LuaScript are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses
[Semantic Versioning](https://semver.org/).

## [3.1.0] - 2026-11-27

A hardening release. Scripts can no longer crash, hang or exhaust the memory of
the interpreter through the compiler, the VM, the core library or the native
modules: each of those paths now raises an ordinary error that `pcall` can
catch. `luascript fmt` no longer changes what a program means, and `luascript
pkg install` refuses a dependency that does not match its locked commit. The
bytecode format is unchanged (`SerialVersion` 3), so binaries made with
`luascript build` on 3.0.0 still run.

### Behavior changes

These follow Lua 5.4. Scripts that relied on the old, looser behavior may now
raise an error.

- `string.format` accepts at most two digits for width and precision, so
  `"%999d"` raises `invalid conversion` instead of building a huge string.
- Integer arguments given a float with a fractional part raise
  `number has no integer representation` instead of being truncated.
- A pattern match that recurses too deeply raises `pattern too complex`
  instead of silently reporting no match. The old behavior made patterns such
  as `("a?"):rep(n) .. ("a"):rep(n)` backtrack for an exponential time.
- `table.remove(t, pos)` checks `pos` the way Lua 5.4 does. On an empty table
  it accepts `0` and `#t + 1`.
- More than 200 coroutines resumed inside each other raises
  `C stack overflow`.
- A coroutine that yields while its `__close` handlers run after it died
  raises `attempt to yield across a C-call boundary`. It used to hang.
- Size limits on native modules: `crypto.password_hash` caps `time`,
  `memory` (1 GiB), `key_length` and `salt_length`, and
  `crypto.password_verify` treats a stored hash with larger values as
  malformed. `queue.channel` capacity is at most 16 777 216.
  `plot.histogram` and `stats.histogram` bins are at most 1 048 576.
  `ndarray.arange` rejects ranges that are too long and steps too small to
  advance.

### Added

- `pairs` honors the `__pairs` metamethod.
- `__call` handlers can themselves be callable tables. The chain is
  followed as in Lua 5.4, and a loop raises an error instead of overflowing
  the stack.

### Fixed

**Compiler**

- A NUL byte in the source no longer ends the script early. The rest of the
  file used to be dropped silently.
- Very deep expressions (long `a.b.c…`, `f()()…`, `a + b + …` chains and
  large interpolated strings) produce a syntax error instead of crashing
  the process with a stack overflow.
- Long `and`/`or`, index, call and method chains compile in linear time. An
  80 000-term chain used to take minutes.
- Recursive generic types such as `type L<T> = { n: L<L<T>> }` no longer
  make the type checker run for an exponential time, and deeply nested
  conditions no longer slow down type narrowing.

**Runtime**

- An uncaught error whose `__tostring` itself fails no longer crashes the
  process. It prints `(error object is a … value)`.
- `string.format` prints `inf`, `-inf` and `nan` the way C and Lua do, not as
  `+Inf` and `NaN`.
- `math.modf` returns an integer argument unchanged with fraction `0.0`,
  and returns fraction `0.0` for infinity.
- `string.byte` refuses ranges of more than 16M bytes instead of allocating
  without bound.
- `table.move` raises `destination wrap around` instead of overflowing.

**Native modules**

- Loading an `ml` model checks its layer sizes, activation, mode and loss,
  and that its weights match the layout. A corrupt or hostile model file used
  to crash the process or allocate unbounded memory.
- `naivebayes:learn` with an unknown class raises an error instead of
  panicking.
- `httpserver`: a handler that returns a status outside 100–999 gets a
  `500` response explaining the bug, instead of a dropped connection.
- `io.lines` and `file:lines` on a handle that is closed or not readable raise
  an error instead of panicking.
- `time` functions called with a missing timestamp argument raise an error instead of
  panicking.
- `stats.quantile` and `stats.percentile` reject `NaN`.

**Formatter**

- `luascript fmt` adds parentheses where the parser's desugaring needs them.
  For example, `x *= a + b` is printed as `x = x * (a + b)`, not
  `x = x * a + b`, and `` `a{b}`:upper() `` keeps its parentheses.
- `- -x` keeps its space instead of becoming a `--` comment.
- Strings are quoted with LuaScript escapes. Go's `\uXXXX` form, which the
  lexer does not read, used to change the string's contents.
- A `return` with no value is no longer joined to the next `match` arm.
- Long call and index chains format in linear time.

### Security

- `luascript pkg install` checks out a locked dependency by its exact commit
  and refuses the install if the fetched commit differs from the lock file.
  A branch or tag named after the pinned SHA could previously replace it.
- The bytecode cache directory is created with mode `0700` instead of `0755`,
  so other users cannot read it.
- Fixed the warnings reported by CodeQL code scanning.

### Dependencies

- Go: `modernc.org/sqlite` 1.62.0, `github.com/mattn/go-sqlite3` 1.14.53,
  `golang.org/x/crypto` 0.59.0, `github.com/go-sql-driver/mysql` 1.10.2,
  `github.com/microsoft/go-mssqldb` 1.12.0, `fyne.io/fyne/v2` 2.8.2.
- Website: Next.js 16.4.6, React 19.3.1, Tailwind CSS 4.3.5, Shiki 4.6.0 and
  smaller updates.
- CI: `softprops/action-gh-release` 3.0.4, `actions/upload-artifact` 7.0.2,
  `actions/download-artifact` 8.0.2.

## [3.0.0] - 2026-10-11

LuaScript 3.0 makes the type checker check table literals, brings coroutines,
long strings and to-be-closed variables in line with Lua 5.4, and moves the
documentation to a new website: https://hilthontt.github.io/LuaScript/

### ⚠ Breaking changes

- **Module path is now `github.com/hilthontt/luascript/v3`.** Go requires the major-version suffix, and its absence is why `go install …@latest` used to install the old 1.x build. Code that imports LuaScript packages must update its imports.
- **Table literals are type-checked.** A literal assigned to an annotated local, passed to a typed parameter, returned from a typed function or used as a default value is now checked against that type. Code the checker used to accept may now fail to compile:
  - a missing required field: `table literal is missing required field "y" of type "number"`
  - a field the record does not declare: `table literal has field "z", which type "Point" does not declare`
  - a wrong field or element type: `Type "string" could not be converted into "number" in field "age"`

  `{}` is still accepted for any record, optional `T?` fields can be left out, and a spread (`{ ...base, x = 1 }`) turns off the missing-field check.
- **Native module signatures match the runtime.** Functions that raise on failure no longer declare an error return (`linalg.inverse`/`solve`, `os.open`/`create`/`mkdir`; `os.hostname` returns a string), and required arguments are no longer optional (`linalg.zeros`/`ones` columns, `ndarray.linspace` count, `os.open` flag and permissions, `os.mkdir` permissions). Calls that relied on the looser declarations now fail to type-check.
- **Long strings drop the newline right after `[[`**, as in Lua 5.4, and line endings inside them become `\n`. `#[[⏎hello]]` is now 5, not 6.
- **Coroutine semantics follow Lua 5.4.** Resuming a running or normal coroutine fails with `cannot resume non-suspended coroutine`, and `coroutine.close` on a coroutine that died with an error returns `false, err`.

### Language and runtime

- `coroutine.status` reports `"normal"` for a coroutine that resumed another one.
- `coroutine.running()` on the main thread returns the main coroutine and `true` instead of `nil`.
- Coroutines print as `thread: 0x…` instead of a raw Go struct.
- `coroutine.create` and `coroutine.wrap` accept Go functions.
- io file handles support `<close>`: `local f <close> = io.open(path)` closes the file at the end of the scope.
- `pcall` and `xpcall` call tables that have a `__call` metamethod.
- `math.deg` and `math.rad` are on the global `math` table.
- `require("ui")` in a build without `-tags luascript_ui` returns a stub with `supported = false`; it used to raise.

### Type checker

- The `os`, `io` and `utf8` globals are typed the same way as their `require()` forms.
- Shadowing `require` (`local require = …`) makes the checker treat its results as `any`.
- `c and x or y` with a boolean `c` types as `x`'s type instead of `boolean | T`.
- Native declarations corrected across clustering, classification, compression, math, log, debug, plugin, time and test; `require("io")` is typed.

### CLI

- `-watch` reports compile and runtime errors and keeps watching. It used to exit on the first error.
- Running a test file directly (`luascript foo_test.lsc`) exits with status 1 when a test fails, including with `-time` and in bundled binaries.
- `luascript -v` reports the real commit and build time. They come from `-ldflags -X` (set by the Makefile and the release workflow), or otherwise from Go's build info; they used to be hardcoded.
- Fixed the `luascript profile` build hint and the `analyze` usage line.

### Plotting

- Bars and histogram bins stay inside the plot frame. The outer bars used to hang half outside it.
- Charts have headroom above the tallest value, and points no longer sit on the frame border.
- A plain string is accepted as a series label: `plot.bar(cats, vals, "stock")`.

### Documentation

- **A new documentation site**, built from `website/` and deployed to GitHub Pages. It has a guide to the language and type system, a standard-library reference, terminal recordings made from real interpreter runs, and a new logo.
- `luascript doc` and DESIGN.md were checked against the runtime and corrected, including channel semantics, dataframe, csv, ml and clustering signatures, and the "deliberately out" list (intersections, literal types and typed native `require` all work).
- The README is shorter and points to the website.

### Other

- Updated Go dependencies, and the VS Code client's `vscode-languageclient`, `@types/node` and TypeScript 7.
- The VS Code client's `build:server` script writes `bin/luascript` on Linux and macOS, where the extension looks for it.

## [2.0.0] - 2026-10-07

A major release: a much richer language surface (destructuring, spread,
`?.` / `??` / `|>`, `impl`, `interface`, literal types), a stricter type
checker, real to-be-closed variables, stack tracebacks, and a broad round of
Lua 5.4 conformance and standard-library work.

### ⚠ Breaking changes

- **Bytecode format bumped (`SerialVersion` 2 → 3).** Bytecode produced by
  v1.x, including binaries made with `luascript build`, must be rebuilt.
- **Enums are typed as their exact value set.** `enum Color RED, GREEN, BLUE`
  is now `1 | 2 | 3`, so passing an out-of-range number such as `paint(99)` is
  a compile error in checked files.
- **`match` exhaustiveness is enforced.** When the subject has a finite type
  (tagged enum, singleton union, classic enum, boolean), a missing arm is a
  compile error. Untyped subjects are not checked, so plain Lua is unaffected.
- **`local x <close>` now actually closes.** Previously the attribute was
  parsed but ignored; now the value must have a `__close` metamethod (or be
  `nil`/`false`), and `__close` runs on scope exit, `break`, `return`,
  errors, `pcall`, `try`/`catch` and closed coroutines.
- **`struct` returns a callable class table** instead of a bare function, so
  `impl` members can be attached. Code that checked `type(MyStruct) ==
  "function"` will see `"table"`.
- **Struct constructors treat a single table as named fields only** when every
  key is a declared field and it has no metatable. Other tables are now passed
  as a positional argument.
- **`tonumber` follows Lua.** Strings with `_` digit separators or
  `inf`/`nan` spellings no longer convert; hex floats without an exponent do,
  and hex integers wrap modulo 2^64.
- **`regexp`: unmatched capture groups return `nil`** instead of `""`.
- **`getmetatable`** now respects the metatable of non-table values and no
  longer returns properties when the metatable is `nil`.
- **`impl` and `interface` are contextual keywords** (like `type`/`struct`).
- **Removed:** the `regression` data-science package, the Bayes classifier's
  file persistence, `std/heap.New`, `std/hashmap.NewHashMap`, and
  `compression.HuffDecode`.
- **Go 1.27** is now required to build from source.

### Added

**Language**

- Table destructuring with defaults, renames and rest:
  `local { host, port = p, timeout or 30, ...rest } = cfg`,
  `local [ first, second, ...tail ] = list`.
- Table spread: `{ ...defaults, ...overrides, verbose = true }`.
- Optional chaining `a?.b`, `a?[k]`, `a?:m()`; nil-coalescing `a ?? b`;
  pipeline `x |> f`, `x |> f(extra)`, `x |> obj:m(extra)`.
- `impl` blocks that attach static and `self` methods to a struct.
- `interface Name { ... }` declarations.
- Generic constraints `<T: Bound>`, checked for both explicit and inferred
  type arguments.
- Intersection types `A & B` (binds tighter than `|`).
- Literal (singleton) types: `type Mode = "read" | "write" | "append"`, with
  narrowing on `==` and widening at inference sites.

**Runtime and tooling**

- Runtime errors carry source positions; uncaught errors print a full stack
  traceback.
- New `test` module (`require("test")`): `test`/`it`, nested `describe`,
  `skip`, `before_each`/`after_each`, and 15 assertions.
- `db` module: MySQL/MariaDB, SQLite (pure-Go and cgo) and SQL Server drivers
  alongside PostgreSQL, with driver-name aliases (`pg`, `mariadb`, `mssql`, …).
- `require("<module>")` with a literal name resolves to that module's type
  (24 native modules typed).

**Lua 5.4 conformance and standard library**

- `_G`, `_VERSION`, `math.ult`, and full `string.pack` / `string.unpack` /
  `string.packsize`.
- `os`, `io` and `utf8` are available as globals without `require`.
- `io.read` supports `"n"`, `"a"` and byte counts.
- `regexp`: `find`, `groups`, `find_all_captures`, `replace_func`.
- `crypto`: argon2id password hashing, generic HMAC, base64url, PBKDF2,
  unbiased `random_int`.
- `http`: JSON/form bodies, `headers_raw`, redirect control, basic auth.
- `stats`: t-tests, normal pdf/cdf, Spearman, histogram, Neumaier summation.
- `linalg`: Cholesky, QR, `lstsq`, `rank`, `eigh`.
- `ndarray`: `sort`, `argsort`, `median`, `cumsum`, `diff`, `any`, `all`,
  `nonzero` and more elementwise operations.

### Changed

- Single-target assignments skip temp-slot staging (faster codegen).
- Bad-argument errors use library-qualified names (`math.sqrt`, not `sqrt`).
- Examples section overhauled.
- Large internal refactor: files split up, module surfaces no longer encoded
  three times, and ~950 lines of dead code removed.
- Dependency updates (sqlite, go-mssqldb, mysql, tcell, x/crypto, CI actions).

### Fixed

- `goto` could loop forever or read the wrong variable.
- Table spread put values after a `nil` in the wrong slot and truncated
  trailing multi-value calls and `...`.
- `luascript fmt` dropped `...` from spread fields, changing program behaviour.
- The lexer read past EOF on a trailing backslash.
- Inefficient string building in hot paths.

## [1.0.0] - 2026-08-06

Initial release.

[3.1.0]: https://github.com/HilthonTT/LuaScript/compare/v3.0.0...v3.1.0
[3.0.0]: https://github.com/HilthonTT/LuaScript/compare/v2.0.0...v3.0.0
[2.0.0]: https://github.com/HilthonTT/LuaScript/compare/v1.0.0...v2.0.0
[1.0.0]: https://github.com/HilthonTT/LuaScript/releases/tag/v1.0.0
