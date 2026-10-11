// Builds the bundled language server into ./bin with the name the extension
// looks for (src/extension.ts): bin/luascript.exe on Windows, bin/luascript
// elsewhere. Run via `bun run build:server` (or `node scripts/build-server.mjs`).
import { spawnSync } from "node:child_process";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const clientDir = join(dirname(fileURLToPath(import.meta.url)), "..");
const exe = process.platform === "win32" ? "luascript.exe" : "luascript";
const out = join("bin", exe);

const result = spawnSync("go", ["build", "-o", out, "../cmd/luascript"], {
  cwd: clientDir,
  stdio: "inherit",
});
if (result.error) {
  console.error(`build:server: ${result.error.message}`);
  process.exit(1);
}
process.exit(result.status ?? 1);
