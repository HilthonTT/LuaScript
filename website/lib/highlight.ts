import { createHighlighter, type Highlighter } from "shiki";

import luascript from "./luascript.tmLanguage.json";

let highlighter: Promise<Highlighter> | undefined;

/**
 * Highlights LuaScript at build time with the same grammar and theme the
 * docs use, so landing-page snippets and docs code blocks match.
 */
export async function highlightLuaScript(code: string): Promise<string> {
  highlighter ??= createHighlighter({
    themes: ["vesper"],
    // The grammar JSON is a valid TextMate grammar; shiki wants its own type.
    langs: [{ ...(luascript as object), name: "luascript" } as never],
  });
  return (await highlighter).codeToHtml(code.trim(), {
    lang: "luascript",
    theme: "vesper",
    colorReplacements: { "#101010": "transparent" },
  });
}
