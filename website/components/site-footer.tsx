import Link from "next/link";

import { site } from "@/lib/site";
import { Logo } from "./logo";

export function SiteFooter() {
  return (
    <footer className="border-t border-white/5">
      <div className="mx-auto flex max-w-7xl flex-col gap-6 px-5 py-10 text-sm text-mist-400 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex flex-wrap items-center gap-4">
          <Logo />
          <span>MIT licensed · written in Go</span>
        </div>
        <div className="flex flex-wrap gap-6">
          <Link href="/docs/" className="hover:text-white">
            Docs
          </Link>
          <Link href="/docs/quick-start/" className="hover:text-white">
            Quick start
          </Link>
          <a href={site.repo} className="hover:text-white">
            GitHub
          </a>
          <a href={`${site.repo}/releases`} className="hover:text-white">
            Releases
          </a>
        </div>
      </div>
    </footer>
  );
}
