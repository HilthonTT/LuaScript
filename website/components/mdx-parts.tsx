import { Info, Lightbulb, TriangleAlert } from "lucide-react";

const callouts = {
  info: { icon: Info, box: "border-info/30 bg-info/5", tint: "text-info" },
  tip: { icon: Lightbulb, box: "border-ok/30 bg-ok/5", tint: "text-ok" },
  warn: {
    icon: TriangleAlert,
    box: "border-warn/30 bg-warn/5",
    tint: "text-warn",
  },
};

export function Callout({
  type = "info",
  children,
}: {
  type?: keyof typeof callouts;
  children: React.ReactNode;
}) {
  const { icon: Icon, box, tint } = callouts[type];
  return (
    <div
      className={`not-prose my-6 flex gap-3 rounded-xl border p-4 text-sm leading-relaxed text-mist-100 ${box}`}
    >
      <Icon className={`mt-0.5 size-4 shrink-0 ${tint}`} />
      <div className="[&_a]:text-brand-light [&_a]:underline [&_code]:rounded [&_code]:bg-white/10 [&_code]:px-1 [&_code]:font-mono [&_p]:my-0 [&_p+p]:mt-2">
        {children}
      </div>
    </div>
  );
}

export function Kbd({ children }: { children: React.ReactNode }) {
  return (
    <kbd className="rounded-md border border-white/15 border-b-white/25 bg-ink-800 px-1.5 py-0.5 font-mono text-[0.8em] text-mist-100">
      {children}
    </kbd>
  );
}
