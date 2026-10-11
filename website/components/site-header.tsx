import Link from "next/link";

import { site } from "@/lib/site";
import { GithubIcon } from "./github-icon";
import { Logo } from "./logo";

const links = [
  { href: "/#features", label: "Features" },
  { href: "/docs/", label: "Docs" },
  { href: "/docs/types/", label: "Types" },
  { href: "/docs/native-modules/", label: "Library" },
];

export function SiteHeader() {
  return (
    <header className="sticky top-0 z-40 border-b border-white/5 bg-ink-900/70 backdrop-blur-xl">
      <div className="mx-auto flex h-16 max-w-7xl items-center justify-between px-4 sm:px-5">
        <Link href="/" aria-label="LuaScript home">
          <Logo />
        </Link>
        <nav className="flex items-center gap-1 text-sm text-mist-300">
          {links.map((l) => (
            <Link
              key={l.href}
              href={l.href}
              className="hidden rounded-lg px-3 py-2 transition hover:bg-white/5 hover:text-white sm:block"
            >
              {l.label}
            </Link>
          ))}
          <Link
            href="/docs/"
            className="rounded-lg px-3 py-2 transition hover:bg-white/5 hover:text-white sm:hidden"
          >
            Docs
          </Link>
          <a
            href={site.repo}
            className="ml-2 flex items-center gap-2 rounded-lg border border-white/10 px-3 py-2 transition hover:border-white/25 hover:text-white"
          >
            <GithubIcon />
            <span className="hidden sm:inline">GitHub</span>
          </a>
        </nav>
      </div>
    </header>
  );
}
