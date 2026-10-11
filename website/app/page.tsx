import Link from "next/link";
import {
  ArrowRight,
  Boxes,
  FlaskConical,
  Layers,
  LineChart,
  Package,
  ShieldCheck,
  Sparkles,
  Terminal,
  Wrench,
  Zap,
} from "lucide-react";

import { CodeTabs, type CodeTab } from "@/components/code-tabs";
import { CopyCommand } from "@/components/copy-command";
import { GithubIcon } from "@/components/github-icon";
import { Media } from "@/components/media";
import { SiteFooter } from "@/components/site-footer";
import { SiteHeader } from "@/components/site-header";
import { highlightLuaScript } from "@/lib/highlight";
import { site } from "@/lib/site";

/** A planet and its moon glowing behind the hero, after the logo. */
function Orbit() {
  return (
    <div
      aria-hidden
      className="pointer-events-none absolute inset-x-0 top-0 -z-10 h-[60rem] overflow-hidden [mask-image:linear-gradient(to_bottom,black_50%,transparent)]"
    >
      <div className="bg-stars absolute inset-0" />
      <div className="absolute top-24 left-1/2 size-[46rem] -translate-x-1/2 rounded-full bg-brand/20 blur-[120px]" />
      <div className="absolute top-10 left-[62%] size-64 rounded-full bg-moon/10 blur-3xl" />
      <div className="absolute top-36 left-1/2 h-[30rem] w-[64rem] -translate-x-1/2 -rotate-12 rounded-[100%] border border-white/[0.05]" />
    </div>
  );
}

function Section({
  id,
  eyebrow,
  title,
  intro,
  children,
}: {
  id?: string;
  eyebrow: string;
  title: React.ReactNode;
  intro?: string;
  children: React.ReactNode;
}) {
  return (
    <section id={id} className="mx-auto max-w-7xl scroll-mt-24 px-5 py-24">
      <p className="font-mono text-xs uppercase tracking-[0.25em] text-brand-light">
        {eyebrow}
      </p>
      <h2 className="mt-3 max-w-3xl text-3xl font-semibold tracking-tight text-white sm:text-4xl">
        {title}
      </h2>
      {intro && (
        <p className="mt-4 max-w-2xl text-lg leading-relaxed text-mist-300">
          {intro}
        </p>
      )}
      <div className="mt-12">{children}</div>
    </section>
  );
}

function Showcase({
  title,
  text,
  media,
  alt,
  href,
  icon: Icon,
}: {
  title: string;
  text: string;
  media: string;
  alt: string;
  href: string;
  icon: React.ComponentType<{ className?: string }>;
}) {
  return (
    <Link
      href={href}
      className="group flex flex-col overflow-hidden rounded-3xl border border-white/[0.07] bg-ink-850 transition hover:border-brand/40"
    >
      <div className="p-7 pb-0">
        <span className="inline-flex rounded-xl bg-white/5 p-2.5 text-brand-light ring-1 ring-white/10">
          <Icon className="size-5" />
        </span>
        <h3 className="mt-4 flex items-center gap-2 text-lg font-semibold text-white">
          {title}
          <ArrowRight className="size-4 -translate-x-1 opacity-0 transition group-hover:translate-x-0 group-hover:opacity-100" />
        </h3>
        <p className="mt-1.5 text-sm leading-relaxed text-mist-300">{text}</p>
      </div>
      <div className="mt-6 flex flex-1 items-end px-3 pb-3">
        <Media
          src={media}
          alt={alt}
          className="transition duration-500 group-hover:scale-[1.01]"
        />
      </div>
    </Link>
  );
}

const capabilities = [
  {
    icon: Layers,
    title: "Lua 5.4 at heart",
    text: "Metatables, coroutines, closures, goto, integer subtypes and the full pattern library behave the way Lua does.",
  },
  {
    icon: ShieldCheck,
    title: "Types cost nothing",
    text: "Annotations are checked at compile time and erased before bytecode. Untyped code is just any.",
  },
  {
    icon: Sparkles,
    title: "Modern syntax",
    text: "?. and ?? for nil, |> pipelines, destructuring, spread, continue, if-expressions and default parameters.",
  },
  {
    icon: Zap,
    title: "Real error handling",
    text: "try / catch / throw as a true protected region, defer for cleanup, and stack tracebacks across modules.",
  },
  {
    icon: Package,
    title: "Batteries included",
    text: "json, http, an HTTP server, crypto, SQL databases, regexp, time, compression, collections and more.",
  },
  {
    icon: LineChart,
    title: "Data science",
    text: "N-d arrays with broadcasting, dataframes, stats, linear algebra, clustering, neural nets and SVG plots.",
  },
  {
    icon: Wrench,
    title: "Tooling built in",
    text: "A formatter, a language server with a VS Code extension, a static analyzer, a profiler and offline docs.",
  },
  {
    icon: Boxes,
    title: "Ship one file",
    text: "luascript build bundles your script and the interpreter into a single executable.",
  },
];

const snippets = [
  {
    id: "structs",
    label: "Structs & impl",
    file: "vec.lsc",
    code: `
struct Vec { x: number, y: number }

impl Vec
    function len(self): number
        return math.sqrt(self.x ^ 2 + self.y ^ 2)
    end
end

local v = Vec(3, 4)
print(v:len())           --> 5.0`,
  },
  {
    id: "nil",
    label: "Nil-safe access",
    file: "config.lsc",
    code: `
local config = { server = { port = 8080 } }

local port = config?.server?.port ?? 3000
local host = config?.server?.host ?? "localhost"
print(\`{host}:{port}\`)    --> localhost:8080

local [ first, ...rest ] = { 10, 20, 30 }
print(first, #rest)       --> 10  2`,
  },
  {
    id: "errors",
    label: "try / catch",
    file: "payload.lsc",
    code: `
local json = require("json")

local function load(src: string)
    try
        local data = json.decode(src)
        print(data.lang, data.stars)
    catch err do
        print("bad payload:", err)
    end
end

load('{"lang": "luascript", "stars": 42}')   --> luascript  42
load('{"lang": ')                            --> bad payload: ...`,
  },
  {
    id: "coroutines",
    label: "Coroutines",
    file: "range.lsc",
    code: `
local function range(n: number)
    return coroutine.wrap(function()
        for i = 1, n do coroutine.yield(i) end
    end)
end

local squares = {}
for i in range(5) do
    if i % 2 == 0 then continue end
    squares[#squares + 1] = i * i
end
print(table.concat(squares, ", "))   --> 1, 9, 25`,
  },
];

export default async function Home() {
  const tabs: CodeTab[] = await Promise.all(
    snippets.map(async (s) => ({
      ...s,
      html: await highlightLuaScript(s.code),
    })),
  );

  return (
    <>
      <SiteHeader />
      <main className="relative isolate overflow-x-clip">
        <Orbit />

        {/* Hero */}
        <section className="mx-auto max-w-7xl px-5 pt-20 pb-12 text-center sm:pt-28">
          <Link
            href="/docs/"
            className="inline-flex items-center gap-2 rounded-full border border-white/10 bg-white/[0.03] px-4 py-1.5 text-xs text-mist-300 transition hover:border-white/25 hover:text-white"
          >
            <span className="size-1.5 rounded-full bg-ok" />
            v2.0 · Lua 5.4 compatible · written in Go
            <ArrowRight className="size-3" />
          </Link>
          <h1 className="mx-auto mt-8 max-w-4xl text-5xl font-semibold tracking-tight text-white sm:text-7xl">
            Lua, with{" "}
            <span className="text-lunar">types that stay out of your way.</span>
          </h1>
          <p className="mx-auto mt-6 max-w-2xl text-lg leading-relaxed text-mist-300 sm:text-xl">
            LuaScript keeps the small, fast language you know and adds
            Luau-style gradual types, structs, tagged enums and pattern
            matching, all checked at compile time and erased before the VM runs
            a single instruction.
          </p>
          <div className="mt-10 flex flex-col items-center gap-4">
            <CopyCommand command={site.install} />
            <div className="flex flex-wrap justify-center gap-3">
              <Link
                href="/docs/quick-start/"
                className="inline-flex items-center gap-2 rounded-xl bg-white px-5 py-2.5 text-sm font-semibold text-ink-900 transition hover:bg-mist-100"
              >
                Get started <ArrowRight className="size-4" />
              </Link>
              <a
                href={site.repo}
                className="inline-flex items-center gap-2 rounded-xl border border-white/10 px-5 py-2.5 text-sm font-semibold text-white transition hover:border-white/30"
              >
                <GithubIcon /> Star on GitHub
              </a>
            </div>
          </div>
        </section>

        <div className="relative mx-auto max-w-5xl px-5">
          <div
            aria-hidden
            className="absolute inset-x-16 top-10 -z-10 h-[80%] rounded-full bg-gradient-to-r from-brand/25 via-brand-deep/40 to-brand/20 blur-3xl"
          />
          <Media
            src="run.gif"
            alt="Writing a typed LuaScript program in an editor and running it in the terminal"
            priority
          />
        </div>

        {/* Showcase */}
        <Section
          id="features"
          eyebrow="Features"
          title={
            <>
              Everything Lua does,{" "}
              <span className="text-lunar">
                plus what you kept reaching for
              </span>
              .
            </>
          }
          intro="Start untyped and add annotations where they pay off. The checker catches mistakes before your program runs, and every recording below is a real run of the interpreter."
        >
          <div className="grid gap-5 lg:grid-cols-2">
            <Showcase
              icon={ShieldCheck}
              href="/docs/types/"
              title="Gradual types"
              text="Annotate a function and the checker holds every call to it. A bad call is a compile error, so nothing runs half-way."
              media="types.gif"
              alt="A strict-mode type error caught at compile time, then fixed"
            />
            <Showcase
              icon={Layers}
              href="/docs/match/"
              title="Tagged enums & match"
              text="Sum types with payloads, destructured by match. Forget a variant and the checker tells you which one."
              media="match.gif"
              alt="A match over a tagged enum failing the exhaustiveness check, then passing"
            />
            <Showcase
              icon={Terminal}
              href="/docs/quick-start/"
              title="A REPL that knows types"
              text="Bare expressions print their value, locals survive between lines, and type errors show up as you type."
              media="repl.gif"
              alt="A LuaScript REPL session"
            />
            <Showcase
              icon={FlaskConical}
              href="/docs/testing/"
              title="Testing built in"
              text="describe, it, hooks and a full set of assertions, with a runner that gives every test file its own VM."
              media="test.gif"
              alt="luascript test running the example test suite"
            />
          </div>

          <div className="mt-5 grid gap-px overflow-hidden rounded-3xl border border-white/[0.07] bg-white/[0.07] sm:grid-cols-2 lg:grid-cols-4">
            {capabilities.map(({ icon: Icon, title, text }) => (
              <div key={title} className="bg-ink-850 p-7">
                <Icon className="size-5 text-brand-light" />
                <h3 className="mt-4 font-semibold text-white">{title}</h3>
                <p className="mt-1.5 text-sm leading-relaxed text-mist-300">
                  {text}
                </p>
              </div>
            ))}
          </div>
        </Section>

        {/* Code */}
        <Section
          eyebrow="The language"
          title="Familiar on the first line, more expressive by the tenth."
          intro="Every snippet here runs as written. The output is in the comments."
        >
          <div className="grid items-start gap-10 lg:grid-cols-[1.35fr_1fr]">
            <CodeTabs tabs={tabs} />
            <ul className="space-y-5 text-mist-300">
              {[
                [
                  "Structs and impl blocks",
                  "Nominal types with positional or named construction, and methods attached with impl.",
                ],
                [
                  "Nil handled in the syntax",
                  "Optional chaining, nil-coalescing and destructuring with rest bindings.",
                ],
                [
                  "Errors without the pcall dance",
                  "try / catch is a real protected region: return and break inside it do what you expect.",
                ],
                [
                  "Still Lua underneath",
                  "Coroutines, closures and the standard library work as they do in Lua 5.4.",
                ],
              ].map(([title, text]) => (
                <li key={title} className="flex gap-4">
                  <span className="mt-1.5 size-2 shrink-0 rounded-full bg-gradient-to-br from-moon to-brand" />
                  <span>
                    <span className="font-semibold text-white">{title}.</span>{" "}
                    {text}
                  </span>
                </li>
              ))}
              <li>
                <Link
                  href="/docs/modern-syntax/"
                  className="inline-flex items-center gap-2 font-semibold text-brand-light hover:underline"
                >
                  Tour the syntax <ArrowRight className="size-4" />
                </Link>
              </li>
            </ul>
          </div>
        </Section>

        {/* Pipeline */}
        <Section
          eyebrow="Under the hood"
          title="A compiler you can read end to end."
          intro="Lex, parse, check, fold, emit, run. The AST is the only contract between stages, and the VM never sees a type."
        >
          <Media
            src="pipeline.svg"
            alt="The LuaScript pipeline: lexer, parser, constcheck, typecheck, optimizer, bytecode generator and stack VM"
          />
          <Link
            href="/docs/architecture/"
            className="mt-8 inline-flex items-center gap-2 font-semibold text-brand-light hover:underline"
          >
            Read the architecture guide <ArrowRight className="size-4" />
          </Link>
        </Section>

        {/* CTA */}
        <section className="mx-auto max-w-7xl px-5 pb-28">
          <div className="relative overflow-hidden rounded-3xl border border-white/[0.07] bg-ink-850 px-6 py-16 text-center sm:px-8">
            <div
              aria-hidden
              className="absolute inset-0 bg-gradient-to-br from-brand/15 via-transparent to-brand-deep/30"
            />
            <h2 className="relative text-3xl font-semibold tracking-tight text-white sm:text-4xl">
              Install LuaScript in under a minute.
            </h2>
            <p className="relative mx-auto mt-4 max-w-xl text-mist-300">
              One command with Go, or grab a prebuilt binary for Linux, macOS or
              Windows from the releases page.
            </p>
            <div className="relative mt-8 flex justify-center">
              <CopyCommand command={site.install} />
            </div>
          </div>
        </section>
      </main>
      <SiteFooter />
    </>
  );
}
