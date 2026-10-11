// rehype-pretty-code preconfigured with the LuaScript grammar. Turbopack only
// accepts serializable MDX options, so next.config.mjs names this module by
// path instead of passing the highlighter function inline.
import rehypePrettyCode from "rehype-pretty-code";
import { bundledLanguages, createHighlighter } from "shiki";

import luascript from "./luascript.tmLanguage.json" with { type: "json" };

export default function rehypeCode() {
  return rehypePrettyCode({
    theme: "vesper",
    keepBackground: false,
    getHighlighter: (options) =>
      createHighlighter({
        ...options,
        langs: [
          { ...luascript, name: "luascript", aliases: ["lsc"] },
          bundledLanguages.lua,
          bundledLanguages.sh,
          bundledLanguages.bash,
          bundledLanguages.powershell,
          bundledLanguages.json,
          bundledLanguages.go,
          bundledLanguages.yaml,
          bundledLanguages.text,
        ],
      }),
  });
}
