export type DocLink = { title: string; href: string };
export type DocSection = { title: string; links: DocLink[] };

/** Order of the docs sidebar and the previous/next links. */
export const docs: DocSection[] = [
  {
    title: "Getting started",
    links: [
      { title: "Introduction", href: "/docs/" },
      { title: "Installation", href: "/docs/installation/" },
      { title: "Quick start", href: "/docs/quick-start/" },
      { title: "CLI & tooling", href: "/docs/cli/" },
    ],
  },
  {
    title: "Language",
    links: [
      { title: "Basics", href: "/docs/basics/" },
      { title: "Functions", href: "/docs/functions/" },
      { title: "Tables & metatables", href: "/docs/tables/" },
      { title: "Strings & patterns", href: "/docs/strings/" },
      { title: "Modern syntax", href: "/docs/modern-syntax/" },
      { title: "Errors, try & defer", href: "/docs/errors/" },
      { title: "Coroutines", href: "/docs/coroutines/" },
      { title: "Modules & require", href: "/docs/modules/" },
    ],
  },
  {
    title: "Types",
    links: [
      { title: "Gradual typing", href: "/docs/types/" },
      { title: "Strict mode", href: "/docs/strict-mode/" },
      { title: "Structs & impl", href: "/docs/structs/" },
      { title: "Enums & tagged unions", href: "/docs/enums/" },
      { title: "Pattern matching", href: "/docs/match/" },
      { title: "Generics & interfaces", href: "/docs/generics/" },
    ],
  },
  {
    title: "Standard library",
    links: [
      { title: "Core library", href: "/docs/stdlib/" },
      { title: "Native modules", href: "/docs/native-modules/" },
      { title: "Jobs & channels", href: "/docs/concurrency/" },
      { title: "Data science", href: "/docs/data-science/" },
      { title: "Testing", href: "/docs/testing/" },
    ],
  },
  {
    title: "Internals",
    links: [
      { title: "Architecture", href: "/docs/architecture/" },
      { title: "Contributing", href: "/docs/contributing/" },
    ],
  },
];

export const allDocs: DocLink[] = docs.flatMap((s) => s.links);
