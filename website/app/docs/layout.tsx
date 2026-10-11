import { DocsPager, DocsSidebar } from "@/components/docs-nav";
import { SiteFooter } from "@/components/site-footer";
import { SiteHeader } from "@/components/site-header";

export default function DocsLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <>
      <SiteHeader />
      <div className="mx-auto grid max-w-7xl gap-10 px-5 lg:grid-cols-[15rem_1fr]">
        <aside className="hidden lg:block">
          <div className="scrollbar-hover sticky top-16 -ml-3 max-h-[calc(100vh-4rem)] overflow-y-auto overscroll-contain py-10 pr-3 pl-3 mask-[linear-gradient(to_bottom,transparent,black_2.5rem,black_calc(100%-2.5rem),transparent)]">
            <DocsSidebar />
          </div>
        </aside>
        <main className="min-w-0 py-10 lg:py-14">
          <details className="mb-8 rounded-xl border border-white/10 p-4 lg:hidden">
            <summary className="cursor-pointer text-sm font-medium text-mist-300">
              Documentation menu
            </summary>
            <div className="mt-4">
              <DocsSidebar />
            </div>
          </details>
          <article className="prose prose-invert max-w-3xl prose-headings:tracking-tight prose-h1:text-4xl prose-h1:font-semibold prose-a:text-brand-light prose-a:no-underline hover:prose-a:underline prose-strong:text-white prose-th:text-mist-100 prose-td:text-mist-300">
            {children}
          </article>
          <div className="max-w-3xl">
            <DocsPager />
          </div>
        </main>
      </div>
      <SiteFooter />
    </>
  );
}
