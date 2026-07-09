---
name: "[BUG]"
about: " Report a lexer/parser/typecheck/codegen/VM bug or wrong runtime behavior"
title: ''
labels: bug
assignees: HilthonTT

---

**Describe the bug**
A clear and concise description of what the bug is.
 
**Stage / component**
Where does it go wrong?
 
- [ ] Lexer
- [ ] Parser
- [ ] Type checker (gradual types)
- [ ] Optimizer / bytecode generator
- [ ] VM (runtime, closures, coroutines, metatables)
- [ ] Native module (name: `______`)
- [ ] REPL
- [ ] Tooling (`fmt` / `build` / `analyze` / `profile`)
**Minimal reproduction**
The smallest `.lsc` snippet that triggers it:
 
```lua
-- your minimal script here
```
 
**How you ran it**
```bash
# e.g. go run ./cmd examples/mycase.lsc
#      go run ./cmd -dis mycase.lsc   (bytecode dump often helps codegen bugs)
```
 
**Expected behavior**
What you expected (output value, or that it should/shouldn't be a compile error).
 
**Actual behavior**
What actually happened. Paste the full output, including any `type-error:` prefix or Go panic + stack trace.
 
```
# Paste output here
```
 
**Mode directive**
Was the file `--!strict`, `--!nonstrict`, `--!nocheck`, or none (default gradual)?
 
**Environment**
- Go version (`go version`):
- OS / arch:
- Commit / branch:
- Build tags (e.g. plain, or `-tags luascript_ui`):
- Relevant env (`LUASCRIPT_LIB`, `package.path` entries) if it's a `require` / module issue:
**Additional context**
- Is this behavior different from stock Lua 5.4? If so, how does `lua5.4` handle the same snippet?
- Note: some things are deliberately out of scope for type-system v1 (generics, intersections, refinements, string-singletons, cross-module type checking, recursive aliases) — see the README "Not in v1" list before filing a typecheck issue.
