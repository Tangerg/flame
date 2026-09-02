// `shiki` is dynamic-imported to keep its ~400KB core and grammar JSONs out of the main
// chunk until a code block first renders.

import type { Highlighter } from "shiki";

const THEMES = ["github-dark", "github-light-high-contrast"] as const;

const LANGS = [
  "typescript",
  "javascript",
  "tsx",
  "jsx",
  "python",
  "go",
  "rust",
  "java",
  "c",
  "cpp",
  "csharp",
  "ruby",
  "php",
  "swift",
  "kotlin",
  "bash",
  "shell",
  "json",
  "yaml",
  "toml",
  "html",
  "css",
  "scss",
  "markdown",
  "sql",
  "diff",
  "dockerfile",
  "graphql",
  "xml",
] as const;

// A long transcript renders hundreds of code blocks, so one line per distinct cause.
const reported = new Set<string>();

/** Highlighting is a decoration — nothing here throws, the block falls back to plain text. */
export function reportHighlightFailure(cause: string, error: unknown): void {
  if (reported.has(cause)) return;
  reported.add(cause);
  console.error(`[highlight] ${cause}:`, error);
}

let promise: Promise<Highlighter> | null = null;

export function getHighlighter(): Promise<Highlighter> {
  if (promise === null) {
    const pending = import("shiki").then(({ createHighlighter }) =>
      createHighlighter({
        themes: [...THEMES],
        langs: [...LANGS],
      }),
    );
    // A rejection is NOT cached: the chunk fetch can fail once, and memoising that leaves the
    // whole session unhighlighted with no way back. Cleared only if the slot still holds THIS
    // attempt — a later caller may already have started a successor.
    const attempt: Promise<Highlighter> = pending.catch((error: unknown) => {
      if (promise === attempt) promise = null;
      throw error;
    });
    promise = attempt;
  }
  return promise;
}

// Maps, not object literals: keyed by a path the person or the model chose, and an object
// answers `constructor` / `toString` / `__proto__` with an inherited value.

const LANG_BY_FILENAME = new Map([
  ["Dockerfile", "dockerfile"],
  ["Makefile", "bash"], // close enough for tab-indented recipes
]);

const LANG_BY_EXTENSION = new Map([
  ["ts", "typescript"],
  ["tsx", "tsx"],
  ["mts", "typescript"],
  ["cts", "typescript"],
  ["js", "javascript"],
  ["mjs", "javascript"],
  ["cjs", "javascript"],
  ["jsx", "jsx"],
  ["py", "python"],
  ["go", "go"],
  ["rs", "rust"],
  ["java", "java"],
  ["c", "c"],
  ["h", "c"],
  ["cc", "cpp"],
  ["cpp", "cpp"],
  ["cxx", "cpp"],
  ["hpp", "cpp"],
  ["cs", "csharp"],
  ["rb", "ruby"],
  ["php", "php"],
  ["swift", "swift"],
  ["kt", "kotlin"],
  ["kts", "kotlin"],
  ["sh", "bash"],
  ["bash", "bash"],
  ["zsh", "bash"],
  ["json", "json"],
  ["jsonc", "json"],
  ["yaml", "yaml"],
  ["yml", "yaml"],
  ["toml", "toml"],
  ["html", "html"],
  ["htm", "html"],
  ["css", "css"],
  ["scss", "scss"],
  ["md", "markdown"],
  ["markdown", "markdown"],
  ["sql", "sql"],
  ["graphql", "graphql"],
  ["gql", "graphql"],
  ["xml", "xml"],
]);

// Tags a model writes in a fence, not extensions — `c++` and `c#` are not file suffixes.
const LANG_BY_ALIAS = new Map([
  ["ts", "typescript"],
  ["js", "javascript"],
  ["py", "python"],
  ["rb", "ruby"],
  ["rs", "rust"],
  ["sh", "bash"],
  ["zsh", "bash"],
  ["yml", "yaml"],
  ["dockerfile", "dockerfile"],
  ["docker", "dockerfile"],
  ["c++", "cpp"],
  ["c#", "csharp"],
  ["cs", "csharp"],
]);

/** "text" when unrecognised. Pass the result through [resolveLang]: a bundled-looking tag
 *  may still not be loaded. */
export function langFromPath(path: string): string {
  const base = path.slice(path.lastIndexOf("/") + 1);
  const byName = LANG_BY_FILENAME.get(base);
  if (byName) return byName;
  const ext = base.slice(base.lastIndexOf(".") + 1).toLowerCase();
  return LANG_BY_EXTENSION.get(ext) ?? "text";
}

/** Shiki throws on a lang it did not load, so an unbundled tag degrades to "text". */
export function resolveLang(highlighter: Highlighter, lang: string): string {
  // Two dozen entries scanned once per file beats building a Set to throw away.
  const loaded = highlighter.getLoadedLanguages();
  if (loaded.includes(lang)) return lang;
  const aliased = LANG_BY_ALIAS.get(lang.toLowerCase());
  if (aliased && loaded.includes(aliased)) return aliased;
  return "text";
}
