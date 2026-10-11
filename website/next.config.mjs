import { fileURLToPath } from "node:url";

import createMDX from "@next/mdx";

// Set NEXT_PUBLIC_BASE_PATH=/LuaScript when deploying to GitHub Pages.
const basePath = process.env.NEXT_PUBLIC_BASE_PATH ?? "";

/** @type {import('next').NextConfig} */
const nextConfig = {
  output: "export",
  basePath,
  trailingSlash: true,
  images: { unoptimized: true },
  pageExtensions: ["ts", "tsx", "md", "mdx"],
};

const withMDX = createMDX({
  options: {
    remarkPlugins: ["remark-gfm"],
    // Highlighting (with the custom LuaScript grammar) lives in
    // lib/rehype-code.mjs: Turbopack needs plugins named by string.
    rehypePlugins: [
      "rehype-slug",
      fileURLToPath(new URL("./lib/rehype-code.mjs", import.meta.url)),
    ],
  },
});

export default withMDX(nextConfig);
