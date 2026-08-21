# Examples

Runnable `.lsc` programs that double as a tutorial series. Most run straight
from the repo root:

```sh
go run ./cmd/luascript examples/01_basics.lsc
```

A few need a little more:

- `08_stdlib.lsc` — see [below](#running-the-module-examples)
- `53_plugin.lsc` — needs cgo on linux/darwin/freebsd (**not Windows**; use WSL)
- `31_ui_module.lsc` — build with `-tags luascript_ui`
- `15_http_module.lsc` — the sections marked `[network]` reach the internet;
  they are wrapped in `pcall`, so the file still runs to completion offline
- `16_httpserver_module.lsc` — set `HTTPSERVER_DEMO=1` to actually bind a port
  and serve. Without it the file demonstrates the handler contract and exits,
  because `:listen` blocks until the process is killed.

## Language core

| File | What it shows |
| ---- | ------------- |
| `01_basics.lsc` | values and types, integer/float subtypes, scope, truthiness, every loop form |
| `02_functions.lsc` | recursion, closures and upvalues, multi-return adjustment, varargs, higher-order functions |
| `03_tables_and_metatables.lsc` | records, arrays, the table library, `__index`/`__newindex`, operator overloading, inheritance chains |
| `04_coroutines.lsc` | the four states, values flowing both ways, `wrap` as an iterator, errors, `close`, pipelines |
| `11_compounds.lsc` | compound assignment operators (`x op= e`) |
| `22_string_interpolation.lsc` | backtick interpolation: `` `hello {name}` `` desugars to `..`; escaping, and the call-shorthand trap |
| `26_patterns.lsc` | full Lua-pattern surface (`%a %d %w` classes, `()` captures, `%b()`, `%f[set]`) |
| `32_defer.lsc` | `defer call()` — LIFO cleanup on return, fall-off-end, and error unwinding |
| `33_typeof_sizeof.lsc` | the `typeof` / `sizeof` builtins — int/float distinction, `__type` hook |
| `49_continue.lsc` | `continue` in `for`/`while`/`repeat`, with the `repeat`/`until` scoping rule |
| `50_if_expressions.lsc` | `local x = if c then a else b` — no `end`, `else` mandatory |
| `51_default_params.lsc` | default parameters, why `false` doesn't trigger them, earlier-param references |
| `52_attributes.lsc` | `<const>` / `<close>` and what the always-on `constcheck` pass rejects |
| `55_try_catch.lsc` | `try` / `catch` / `throw` — a real protected region, not a `pcall` desugar |

## Types

| File | What it shows |
| ---- | ------------- |
| `05_types.lsc` | the full Luau-style surface — primitives, optionals, unions, **literal (singleton) types**, function types, aliases, narrowing, assertions |
| `06_strict_mode.lsc` | the three mode directives, what `--!strict` adds, and what it does not |
| `42_structs.lsc` | `struct Name { field: T }` — nominal product types, positional/named construction, nesting |
| `43_tagged_enums.lsc` | tagged sum types — payload constructors, nullary singletons, `__tag`/`typeof`, a `Result` pattern |
| `29_enums.lsc` | bare `enum Name V1, V2 end` — int auto-increment, frozen via `__newindex` proxy, typed as the exact literal union of its values |
| `14_match.lsc` | `match` basics — value/literal patterns, multi-pattern arms, `_` wildcard |
| `44_match.lsc` | `match` v2 — typed bindings, `if` guards, enum/struct destructuring, **exhaustiveness checking**, a `Result` pipeline |
| `45_generics.lsc` | generics — parametric functions with inference (`map`/`filter`), `Box<T>`, a `Stack<T>` |
| `46_generics.lsc` | generics, continued — deeper inference and instantiation cases |

## Modules and imports

| File | What it shows |
| ---- | ------------- |
| `07_modules.lsc` | `require`, `package.path`, `package.loaded`, `searchpath` — imports `mathx.lsc` next to it |
| `08_stdlib.lsc` | a bundled-library set loaded via `LUASCRIPT_LIB` — flat modules, dotted submodules, package `init` files |
| `09_native_module.lsc` | a host-provided native module (`native/stdlib/db`) |
| `10_os_module.lsc` | a host-provided native module (`native/stdlib/os`) |

## Standard library

| File | What it shows |
| ---- | ------------- |
| `12_math_module.lsc` | `math` — and the fact that it exists **twice** with different surfaces (the global lacks `cbrt`/`clamp`/hyperbolics; the module lacks `type`/`maxinteger`/`randomseed`) |
| `36_math.lsc` | `math` in depth — Lua 5.4 scalar surface plus `mean`/`variance`/`standard_deviation`/`softmax` |
| `13_json_module.lsc` | `json` — encode/decode, indenting, and the two places JSON and Lua disagree (array-vs-object, `null` vs `nil`) |
| `15_http_module.lsc` | the `http` client — shortcuts, `http.request{...}`, reusable clients, multi-valued headers |
| `16_httpserver_module.lsc` | `httpserver` — the handler contract, why it is serialised, `:listen` / `:stop` |
| `17_crypto_module.lsc` | `crypto` — hashes, HMAC, password hashing, PBKDF2, constant-time comparison, encodings, secure randomness |
| `18_time_module.lsc` | `time` — wall vs monotonic clocks, layouts, parsing, formatting, sleeping |
| `19_regexp_module.lsc` | `regexp` (Go regex; `:capture`, not `:match`) |
| `20_uuid_module.lsc` | `uuid` |
| `21_sort_module.lsc` | `sort` — `sort` / `stable` / `reverse` / `is_sorted` |
| `23_io.lsc` | the full Lua-5.4 `io` library — file handles, `:read`/`:write`/`:lines`/`:seek`, the standard streams, `io.input`/`io.output` |
| `24_bit_utf8.lsc` | `bit32` (fields, rotates, byte order) and `utf8` (bytes vs characters, `offset`, `charpattern`) |
| `25_os_full.lsc` | the expanded `os` surface (`date`, `time`, `clock`, `execute`, `rename`, `tmpname`, `setlocale`) |
| `27_debug_module.lsc` | `debug` — `traceback`, `getinfo`, hook stubs |
| `58_log_module.lsc` | `log` — levels and thresholds, prefixes, redirecting output, and why `log.fatal` exits the process |
| `28_compression_module.lsc` | `compression` — gzip, zlib, deflate, Huffman, run-length |
| `30_std_module.lsc` | `std` — stack, queue, deque, set, list, heap (requires `cmp`), hashmap, trie, B-tree |
| `54_queue_module.lsc` | `queue` — priority job queue (delays, retries, backpressure, metrics) and channels |
| `31_ui_module.lsc` | the `ui` desktop module (Fyne). Run with `-tags luascript_ui` |
| `56_testing.lsc` | `test` — describe/test/it/skip, `before_each`/`after_each`, the full assertion surface, `fail` (one test fails on purpose) |
| `57_db_module.lsc` | `db` — SQL via database/sql. Runs against in-process SQLite, so **no server needed** |
| `53_plugin.lsc` | `plugin` — load Go packages at run time. **cgo + linux/darwin/freebsd only** |

## Tests

`tests/` holds a real suite rather than a walkthrough — it is what
`luascript test` discovers and runs:

```sh
go run ./cmd/luascript test examples/tests        # summary only
go run ./cmd/luascript test -v examples/tests     # every test
go run ./cmd/luascript test -run "rounds" examples/tests
go run ./cmd/luascript test -list examples/tests
```

| File | What it shows |
| ---- | ------------- |
| `tests/math_test.lsc` | describe groups, `before_each`, `assert_near`, `assert_error` with a message pattern |
| `tests/table_test.lsc` | nested describes inheriting hooks, deep equality, string and pattern assertions |

Every `*_test.lsc` file runs in its own VM, so one file cannot leak globals into
the next. `56_testing.lsc` covers the other direction: a test file is an
ordinary chunk, so running one directly executes its tests too — you just don't
get a summary.

## Data science

| File | What it shows |
| ---- | ------------- |
| `34_clustering_module.lsc` | `clustering` — k-means (k-means++ seeding), DBSCAN, hierarchical, mean-shift |
| `35_classification_module.lsc` | `classification` — Naive Bayes (text), KNN, perceptron, logistic regression, SVM |
| `37_stats_module.lsc` | `stats` — median, mode, quantiles, iqr, covariance, correlation, skew/kurtosis, describe, the three means, population vs sample, histograms, the normal distribution, t-tests |
| `38_linalg_module.lsc` | `linalg` — dot, norm, matmul, transpose, det, inverse, solve, rank, least squares, QR, Cholesky, eigendecomposition |
| `39_csv_module.lsc` | `csv` — parse/stringify/read/write, header + numeric coercion, custom delimiters |
| `40_dataframe_module.lsc` | `dataframe` — select/filter/with_column/sort/group_by/describe, building from rows or CSV, pretty `print` |
| `41_ml_module.lsc` | `ml` — a feed-forward neural network (topology, training, prediction, persistence) |
| `47_ndarray_module.lsc` | `ndarray` — dense N-D arrays, broadcasting, operators, axis reductions, matmul, concat |
| `48_plot_module.lsc` | `plot` — dependency-free SVG charting: line, bar, scatter, histogram. Writes `scratch_*.svg` into the working directory (gitignored) |

## Running the module examples

`require` resolves a module name against `package.path`. Two entry kinds matter
here, searched in this order:

1. **The directory of the script being run** — added automatically, so a module
   next to your script is always found regardless of where you launched from.
   This is why `07_modules.lsc` just works.

2. **`LUASCRIPT_LIB`** — a bundled-library root read once at startup, *not* on
   the path unless you set it. `08_stdlib.lsc` is the demo: its modules live
   under `examples/stdlib/`, not next to the script. The example self-bootstraps
   `package.path` so it also runs without the env var, but the canonical
   invocation is:

   ```sh
   # bash
   LUASCRIPT_LIB=./examples/stdlib go run ./cmd/luascript examples/08_stdlib.lsc
   # PowerShell
   $env:LUASCRIPT_LIB="./examples/stdlib"; go run ./cmd/luascript examples/08_stdlib.lsc
   ```

   `LUASCRIPT_LIB` resolves relative to your working directory — adjust if you
   run from somewhere other than the repo root.

Plain cwd-relative entries (`./?.lsc`, `./src/?.lsc`, …) are searched as well,
so a module under your working directory is found even when it sits nowhere near
the script. Native-module examples pull their modules from the host via
`package.preload`, so they need neither a path entry nor `LUASCRIPT_LIB`.
