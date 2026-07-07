# LuaScript for VS Code

Language support for [LuaScript](https://github.com/hilthontt/luascript) (`.lsc`):
diagnostics, hover, completion, document symbols, and formatting — powered by the
`luascript lsp` language server (a Go binary that speaks JSON-RPC over stdio).

## Architecture

```
VS Code  ──stdio (JSON-RPC)──►  luascript lsp   (Go binary, ../cmd/luascript)
  extension.ts (client)              server.go
```

The extension is a thin **client**: it launches the server binary and forwards
LSP traffic. All language intelligence lives in the Go server. See VS Code's
[Language Server Extension Guide](https://code.visualstudio.com/api/language-extensions/language-server-extension-guide).

## Develop / run it

```sh
bun install            # install client deps (once)
bun run build:server   # go build -> ./bin/luascript.exe
bun run compile        # tsc src -> ./out
```

Then press **F5** in VS Code (the "Run Extension" launch config) to open an
Extension Development Host. Open any `.lsc` file to activate the extension.

- `bun run watch` recompiles the TypeScript on save.
- After changing the **Go server**, re-run `bun run build:server` and reload the
  Extension Development Host (`Ctrl+R`).

## Settings

- `luascript.server.path` — absolute path to a `luascript` executable to use as
  the server. Leave empty to use the binary bundled in `./bin`.

## Package it (`.vsix`)

```sh
bun x @vscode/vsce package
```

`vscode:prepublish` rebuilds the server binary and compiles the client first.
