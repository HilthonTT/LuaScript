# Summary

<!-- What does this change do, and why? One or two sentences is fine. -->

Fixes #

## Pipeline stage(s) touched

<!-- Tick everything the change reaches; it tells reviewers which tests matter. -->

- [ ] Lexer / token
- [ ] Parser / AST
- [ ] constcheck
- [ ] Type checker
- [ ] Optimizer (constant folding)
- [ ] Bytecode generator / serialization / cache
- [ ] VM (runtime, closures, coroutines, metatables)
- [ ] Native module (name: `______`)
- [ ] REPL
- [ ] Tooling (`fmt` / `build` / `analyze` / `profile` / `doc`)
- [ ] Docs registry (`internal/docs`)
- [ ] Editor client (`client/`)
- [ ] CI / build / repo chores

## Language surface

- [ ] No change to the language surface.
- [ ] New/changed syntax or semantics — described below, with an example.
- [ ] New/changed stdlib or native-module surface — documented in `internal/docs`
      (`luascript doc -audit` is clean).

```lua
-- Example of the new behaviour, if any.
```

## Checklist

- [ ] `go build ./...`
- [ ] `go vet ./...`
- [ ] `gofmt -l .` reports nothing (or `make fmt`)
- [ ] `go test ./...` passes
- [ ] Tests added or updated for the change (bytecode tests assert exact opcode
      sequences — update them deliberately, not to make red go green)
- [ ] `CLAUDE.md` / `README.md` updated if the architecture or surface changed

## Notes for reviewers

<!-- Anything non-obvious: a deliberate trade-off, a path CI can't exercise
     (REPL, httpserver, bonsai alt-screen, -tags luascript_ui, plugin on WSL), etc. -->
