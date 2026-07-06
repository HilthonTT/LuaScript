import * as path from "path";
import { workspace, window, type ExtensionContext } from "vscode";
import {
  LanguageClient,
  TransportKind,
  type Executable,
  type LanguageClientOptions,
  type ServerOptions,
} from "vscode-languageclient/node";

let client: LanguageClient | undefined;

export async function activate(context: ExtensionContext): Promise<void> {
  // The server is a Go binary that speaks JSON-RPC over stdio (`luascript lsp`).
  // Resolve it from the `luascript.server.path` setting, otherwise fall back to
  // the binary bundled beside the extension in ./bin.
  const config = workspace.getConfiguration("luascript");
  const configured = config.get<string>("server.path")?.trim();
  const bundled = context.asAbsolutePath(
    path.join("bin", process.platform === "win32" ? "luascript.exe" : "luascript"),
  );
  const command = configured && configured.length > 0 ? configured : bundled;

  // stdio transport: VS Code launches `command lsp` and pipes JSON-RPC over the
  // process's stdin/stdout. The server logs to stderr, so it never corrupts the
  // protocol channel.
  const executable: Executable = {
    command,
    args: ["lsp"],
    transport: TransportKind.stdio,
    options: { env: process.env },
  };

  const serverOptions: ServerOptions = {
    run: executable,
    debug: executable,
  };

  const clientOptions: LanguageClientOptions = {
    // Must match the language id contributed in package.json.
    documentSelector: [{ scheme: "file", language: "luascript" }],
    synchronize: {
      fileEvents: workspace.createFileSystemWatcher("**/*.lsc"),
    },
    outputChannelName: "LuaScript Language Server",
  };

  client = new LanguageClient(
    "luascript",
    "LuaScript Language Server",
    serverOptions,
    clientOptions,
  );

  try {
    await client.start();
  } catch (err) {
    window.showErrorMessage(
      `LuaScript language server failed to start (${command}): ${String(err)}`,
    );
  }
}

export async function deactivate(): Promise<void> {
  await client?.stop();
  client = undefined;
}
