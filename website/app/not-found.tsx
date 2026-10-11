import Link from "next/link";

import { SiteFooter } from "@/components/site-footer";
import { SiteHeader } from "@/components/site-header";

export default function NotFound() {
  return (
    <>
      <SiteHeader />
      <main className="mx-auto flex min-h-[60vh] max-w-3xl flex-col items-center justify-center px-5 py-24 text-center">
        <p className="font-mono text-sm text-brand-light">
          luascript: attempt to index a nil value
        </p>
        <h1 className="mt-4 text-4xl font-semibold tracking-tight text-white">
          This page does not exist.
        </h1>
        <Link
          href="/"
          className="mt-8 rounded-xl bg-white px-5 py-2.5 text-sm font-semibold text-ink-900 hover:bg-mist-100"
        >
          Back home
        </Link>
      </main>
      <SiteFooter />
    </>
  );
}
