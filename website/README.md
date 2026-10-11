# LuaScript website

The documentation site at <https://hilthontt.github.io/LuaScript/>: a
Next.js 16 app exported as static HTML and deployed to GitHub Pages by
[`.github/workflows/website.yml`](../.github/workflows/website.yml) on every
push to `main` that touches `website/`.

```sh
npm install
npm run dev      # http://localhost:3000
npm run build    # static export to out/
npm run lint     # tsc --noEmit
```

To preview the Pages build locally, set the base path the workflow uses:
`NEXT_PUBLIC_BASE_PATH=/LuaScript npm run build && npx serve out`.

## Layout

| Path | What it holds |
| --- | --- |
| `app/page.tsx` | The landing page |
| `app/docs/**/page.mdx` | One MDX page per docs topic |
| `lib/docs.ts` | Sidebar order and previous/next links: add new pages here |
| `lib/luascript.tmLanguage.json` | The TextMate grammar used to highlight ` ```luascript ` fences |
| `lib/rehype-code.mjs` | rehype-pretty-code wired to that grammar |
| `components/` | Header, footer, docs nav, `Media`, `Callout`, `Kbd`, `CodeTabs` |
| `public/logo.svg` | The logo; `favicon.svg` is a copy |
| `public/media/` | GIFs, screenshots, plots and the pipeline diagram |
| `scripts/media/` | The Go program that renders the media |

In MDX, `<Media src="run.gif" alt="…" caption="…" />`,
`<Callout type="info|tip|warn">…</Callout>` and `<Kbd>…</Kbd>` are available
without imports. Code fences take a title: ` ```luascript title="hello.lsc" `.

## Media

The terminal GIFs, the traceback screenshot, the plot SVGs and the logo PNGs
(`logo.png`, `apple-touch-icon.png`, `og.png`) are generated, not drawn:

```sh
npm run media                       # or `make media` from the repo root
go run ./scripts/media -only repl   # one scene: logo, run, types, match, repl, test, traceback, plots
```

The generator builds the interpreter from the repo, runs every scene's program
through it, and records the real output, so a recording cannot drift from the
language. Re-render after changing anything a scene shows (error messages, the
REPL banner, the test runner's output). It is a separate Go module
(`website/go.mod`) so its rendering dependencies stay out of the interpreter's
`go.mod`. `pipeline.svg` is hand-written.

The REPL scene is the one approximation: readline cannot read piped input, so
each value is computed by running the same code and is printed the way the
REPL's `printResults` does.
