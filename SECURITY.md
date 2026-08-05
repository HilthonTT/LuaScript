# Security Policy

## Supported versions

LuaScript is pre-1.0. Only the tip of `main` receives security fixes; there are
no maintained release branches yet.

| Version | Supported |
| ------- | --------- |
| `main`  | ✅        |
| tagged releases | latest tag only |

## Reporting a vulnerability

**Please do not open a public issue for a security problem.**

Report it privately through GitHub:
[**Report a vulnerability**](https://github.com/HilthonTT/sakura-lang/security/advisories/new)
(Security → Advisories → Report a vulnerability).

If that is not available to you, email <hans.tandt@gmail.com> with `[luascript
security]` in the subject.

Please include:

- the affected component (VM, parser, a native module, the bundler, …),
- a minimal `.lsc` script or Go test that reproduces it,
- the commit or tag you tested,
- OS/arch, Go version, and any build tags (`luascript_ui`, cgo).

You can expect an acknowledgement within 7 days and a status update within 30
days. Please give us a reasonable window to ship a fix before disclosing
publicly; credit in the advisory is offered by default.

## Threat model — what counts as a vulnerability

LuaScript is an interpreter, and several of its features **execute arbitrary
code by design**. Whether a report is a vulnerability depends on which side of
that line it falls.

In scope:

- Memory-unsafety, panics, or hangs in the compiler or VM reachable from a
  `.lsc` script — a malformed script crashing the process, an unbounded loop in
  the pattern engine, a serialized-bytecode chunk that corrupts VM state.
- Escapes from a Lua-level sandbox the runtime claims to provide.
- Vulnerabilities in bundled native modules: path traversal or injection in
  `io`/`os`/`db`/`http`/`httpserver`, weak or misused primitives in `crypto`,
  decompression bombs in `compression`.
- Insecure handling of the bytecode cache or bundled-`.exe` payload (e.g. a
  cache entry that lets one user's code run under another's).
- Vulnerable dependencies with a reachable path from this code.

Out of scope (documented behaviour, not bugs):

- A script doing what the API allows: `os.execute`, `io.open`, `db` queries,
  `plugin.generate` compiling and loading Go code. **Running an untrusted
  `.lsc` script is equivalent to running an untrusted program.** The runtime is
  not a sandbox and does not claim to be.
- Spec validation in `plugin` (identifier/import-path checks) being bypassed —
  it exists to turn typos into Lua errors, not as a security boundary.
- Findings that require an attacker to already control the machine, the Go
  toolchain, or the interpreter binary.
