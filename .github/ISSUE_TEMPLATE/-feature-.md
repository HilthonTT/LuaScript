---
name: "[FEATURE]"
about: Suggest a language feature, native module, or tooling improvement
title: ''
labels: enhancement
assignees: HilthonTT

---

**Is your feature request related to a problem? Please describe.**
A clear and concise description of what the problem is. Ex. I'm always frustrated when [...]
 
**Describe the solution you'd like**
A clear and concise description of what you want to happen.
 
**Area**
What does this touch?
 
- [ ] Syntax / parser
- [ ] Type system
- [ ] Bytecode / VM semantics
- [ ] Standard library
- [ ] Native module (existing or new: `______`)
- [ ] Tooling (`fmt` / `build` / `analyze` / `profile` / REPL)
**Prior art**
How do Lua 5.4 and/or Luau handle this? Link the relevant reference if there is one — the project tracks Lua 5.4 semantics and Luau's type-system shape.
 
**Sketch**
Example of the proposed syntax or API in `.lsc`:
 
```lua
-- how it would look
```
 
**Describe alternatives you've considered**
A clear and concise description of any alternative solutions or features you've considered.
 
**Additional context**
- If this is a type-system feature, note that generics, intersections, refinements, string-singletons, cross-module type checking, and recursive aliases are explicitly deferred past v1 — say whether this is one of those or something new.
- Anything else: performance implications, whether types can stay fully erased before bytecode, etc.
