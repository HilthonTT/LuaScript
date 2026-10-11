import type { MDXComponents } from "mdx/types";
import Link from "next/link";

import { Media } from "@/components/media";
import { Callout, Kbd } from "@/components/mdx-parts";

/**
 * Markdown links render as plain <a>, which skips the GitHub Pages base path.
 * Site-internal links go through next/link so they pick it up.
 */
function A({ href = "", ...props }: React.ComponentProps<"a">) {
  if (href.startsWith("/")) return <Link href={href} {...props} />;
  return <a href={href} {...props} />;
}

export function useMDXComponents(components: MDXComponents): MDXComponents {
  return { ...components, a: A, Media, Callout, Kbd };
}
