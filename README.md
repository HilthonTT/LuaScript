# sakura-lang

A Lua-flavored language with a stack-based virtual machine and **Luau-style gradual types**, written in Go.

The surface syntax tracks **Lua 5.4** as closely as possible — the same chunks, the same scoping rules, the same metatables, coroutines, and standard library shape. Optional type annotations on top, à la [Luau](https://luau.org), check at compile time and erase before bytecode so the runtime is unchanged.

The implementation is a clean-room rewrite focused on being readable end-to-end: lex → parse → typecheck → bytecode → stack VM. No LLVM, no JIT, no surprises.

## Status

- **Lexer** — Lua 5.4 tokens, long-bracket strings/comments, hex/exponent numbers, `--!strict` / `--!nonstrict` / `--!nocheck` mode directives.
- **Parser** — full Lua 5.4 grammar including `goto`/labels, attributes (`<const>`, `<close>`), method-call sugar, numeric and generic `for`, plus Luau type-syntax (annotations, type aliases, type assertions, optionals, unions, function types, structural table types).
- **Type checker** — gradual: untyped code is implicitly `any`; annotations opt in. Primitives, function types, optionals, unions, type aliases (including structural table shapes), type assertions. Stdlib has hand-written signatures so `math.sqrt(true)` is a compile error.
- **Bytecode** — stack-based with closure upvalues, vararg passing, generic-`for` iteration, and a one-time scan that fills `NumLocals` at runtime where the generator left it blank. Types are erased before this stage — the VM never sees them.
- **VM** — closures, metatables, coroutines (via goroutines + channels), `pcall`/`error` unwinding.
- **Standard library** — `print`/`tostring`/`tonumber`, `ipairs`/`pairs`/`next`, `pcall`/`assert`/`error`, raw and metatable helpers, plus `math`, `string` (no patterns yet), `table`, `io.write`/`read`, `coroutine`, and `package`/`require`.
- **REPL** — readline-driven, history-backed, with continuation prompts for incomplete input. Top-level `local` declarations persist across REPL chunks (a deliberate convenience deviation from `lua`). Type-check errors are surfaced with a distinct `type-error:` prefix.

## Quick start

```sh
# Run the REPL
go run .

# Run a script
go run . examples/05_types.sakura

# Force the REPL even when a script is supplied
go run . -i examples/05_types.sakura

# Print version
go run . -v
```

Build a binary:

```sh
go build -o sakura .
./sakura examples/01_basics.sakura
```

## Bonsai mode

For a break from the language work, `sakura` ships with a small ASCII-bonsai grower. It is unrelated to the Lua runtime — just a fun side mode.

```sh
# Grow a tree in the alt-screen (press q or Ctrl+C to leave)
./sakura -bonsai

# Print a single tree to stdout instead
./sakura -bonsai -bonsai-print

# Animate growth step-by-step
./sakura -bonsai -bonsai-live

# Reproducible tree from a seed
./sakura -bonsai -seed 42

# Attach a message next to the tree
./sakura -bonsai -bonsai-msg "hello, world"
```

| Flag            | Effect                                                                 |
| --------------- | ---------------------------------------------------------------------- |
| `-bonsai`       | Grow an ASCII bonsai tree and exit.                                    |
| `-seed N`       | RNG seed for reproducible trees (`0` = random).                        |
| `-bonsai-print` | Print the tree to stdout instead of staying in the alt-screen.        |
| `-bonsai-live`  | Animate growth step-by-step.                                          |
| `-bonsai-msg S` | Attach a message next to the tree.                                    |

## Examples

A walk-through set lives in `examples/`. Each file is runnable with `sakura examples/<file>`:

| File | What it shows |
| ---- | ------------- |
| `01_basics.sakura` | variables, control flow, primitive values, logical operators |
| `02_functions.sakura` | recursion, closures and upvalues, multi-return, higher-order functions |
| `03_tables_and_metatables.sakura` | records, arrays, methods, operator overloading via `__add`, `__index` |
| `04_coroutines.sakura` | `coroutine.create` / `resume` / `yield` / `wrap` |
| `05_types.sakura` | the full Luau-style type surface — primitives, optionals, unions, function types, type aliases, type assertions |
| `06_strict_mode.sakura` | `--!strict` enforcement and what it rejects |

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

sakura's type system is **gradual** in the Luau sense: annotations are optional, untyped code is treated as `any`, and `any` flows into and out of any typed slot.

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

| Directive       | Effect                                                                 |
| --------------- | ---------------------------------------------------------------------- |
| (none)          | Default. Gradual checking.                                             |
| `--!strict`     | Implicit-any parameters become errors.                                 |
| `--!nonstrict`  | Same as the default. Useful for explicitness.                          |
| `--!nocheck`    | Skip the type pass for this file entirely.                             |

### Not in v1 (deliberately)

- Generics (`function f<T>(x: T): T`)
- Intersection types (`A & B`)
- Type refinements (narrowing inside `if type(x) == "string"`)
- String-singleton types (`"foo" | "bar"`)
- Cross-module type checking — `require()` returns `any`
- Recursive type aliases (the parser accepts them; the resolver doesn't)

These are explicitly named in error messages where relevant, so users hit a clear wall instead of silent miscompiles.

## REPL

Launch with `go run .` (no arguments). Built-in commands:

| Command       | Effect                                            |
| ------------- | ------------------------------------------------- |
| `help`        | print the help screen                             |
| `exit`, `quit`| leave the REPL                                    |
| `reset`       | rebuild the VM (clears all globals and user state)|
| `clear`       | clear the screen                                  |

Key bindings: **Ctrl+C** cancels the current input, **Ctrl+D** exits, **Ctrl+R** searches history.

Bare expressions print their value:

```
 sakura » 1 + 2
=> 3
 sakura » {1, 2, 3}
=> table: 0xc000...
```

Top-level `local` persists across REPL chunks (it's promoted to a global at compile time so subsequent inputs can read it):

```
 sakura » local greeting = "hi"
 sakura » print(greeting)
hi
```

Inside any nested scope (`do`/`if`/`for`/function body) `local` keeps standard Lua semantics.

Incomplete input opens a continuation prompt:

```
 sakura » function double(x)
   …      return x * 2
   …    end
 sakura » print(double(21))
42
```

Type errors land with a distinct prefix so they're easy to spot:

```
 sakura » local x: number = "hi"
type-error: Type "string" could not be converted into "number" at line 1
```

## Project layout

```
.
├── compiler/
│   ├── lexer/         token stream from source text (FSM-driven)
│   ├── token/         token types and keyword table
│   ├── parser/        recursive-descent parser, Pratt-style for expressions
│   ├── ast/           AST node definitions (statements, expressions, types)
│   ├── typecheck/     gradual type system — Type representation, env, pass
│   ├── bytecode/      AST → instruction-set generator
│   └── compiler.go    top-level pipeline (lex → parse → typecheck → bytecode)
├── vm/                stack VM, closures, metatables, coroutines, stdlib
├── repl/              interactive REPL (readline + engine wrapper)
├── examples/          runnable .sakura programs that double as tutorials
├── version/           version string
└── main.go            CLI entrypoint
```

The compiler is designed so each stage is independently testable and the AST is the only contract between parser, type checker, and bytecode generator. The VM never sees source text or types; the parser never sees instructions.

## Non-goals (for now)

- Lua patterns (`string.match` / `gmatch` / `gsub`) — `string.find` is plain-substring only.
- `io.open` and the file-handle stdlib.
- The `debug` and `os` libraries.
- Garbage-collection metamethods (`__gc`, `__close` enforcement).

These are deliberate omissions, not bugs — they're listed so contributors know what's out of scope rather than wondering whether to file an issue.

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
