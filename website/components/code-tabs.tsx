"use client";

import { useState } from "react";

export type CodeTab = { id: string; label: string; file: string; html: string };

/** Pre-highlighted snippets (see lib/highlight.ts) behind a row of tabs. */
export function CodeTabs({ tabs }: { tabs: CodeTab[] }) {
  const [active, setActive] = useState(tabs[0].id);
  const tab = tabs.find((t) => t.id === active) ?? tabs[0];
  return (
    <div className="overflow-hidden rounded-3xl border border-white/[0.07] bg-ink-950">
      <div
        role="tablist"
        className="flex gap-1 overflow-x-auto border-b border-white/[0.07] bg-ink-850 p-2 [scrollbar-width:none]"
      >
        {tabs.map((t) => (
          <button
            key={t.id}
            type="button"
            role="tab"
            aria-selected={t.id === active}
            onClick={() => setActive(t.id)}
            className={`shrink-0 rounded-lg px-3.5 py-1.5 text-sm transition ${
              t.id === active
                ? "bg-brand/20 font-medium text-brand-light"
                : "text-mist-400 hover:bg-white/5 hover:text-white"
            }`}
          >
            {t.label}
          </button>
        ))}
      </div>
      <div className="px-6 pt-4 font-mono text-xs text-mist-400">
        {tab.file}
      </div>
      <div
        className="overflow-x-auto px-6 pt-3 pb-6 font-mono text-[0.85rem] leading-7 [&_pre]:!bg-transparent"
        dangerouslySetInnerHTML={{ __html: tab.html }}
      />
    </div>
  );
}
