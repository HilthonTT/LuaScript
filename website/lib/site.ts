export const site = {
  name: "LuaScript",
  tagline: "Lua 5.4 with gradual types",
  repo: "https://github.com/HilthonTT/LuaScript",
  url: "https://hilthontt.github.io/LuaScript/",
  // The /v3 suffix is part of the module path; without it Go installs 1.x.
  install: "go install github.com/hilthontt/luascript/v3/cmd/luascript@latest",
};

/** Prefixes public assets with the deploy base path (GitHub Pages). */
export function asset(path: string): string {
  const base = process.env.NEXT_PUBLIC_BASE_PATH ?? "";
  return `${base}${path.startsWith("/") ? path : `/${path}`}`;
}
