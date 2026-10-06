# Changelog

All notable changes to LuaScript are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses
[Semantic Versioning](https://semver.org/).

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

[2.0.0]: https://github.com/HilthonTT/LuaScript/compare/v1.0.0...v2.0.0
[1.0.0]: https://github.com/HilthonTT/LuaScript/releases/tag/v1.0.0
