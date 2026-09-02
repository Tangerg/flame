// A lazy singleton: `createHighlighter` is async and loads grammars from bundled JSON, so
// it is created once on first request and shared app-wide.
//
// The `shiki` module is ALSO dynamic-imported, keeping its ~400KB core and grammar JSONs
// out of the main chunk until a code block first renders.

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

// A transcript renders one code block per fenced section, so reporting every failure would
// put the same line in the console hundreds of times on a long conversation — which is how a
// diagnostic becomes noise nobody reads. One line per distinct cause is enough to find it.
const reported = new Set<string>();

/** Highlighting is a decoration: when it fails the block still renders, as plain text. That
 *  is the right fallback and the reason nothing here throws — but a failure that leaves no
 *  trace at all is one nobody can act on. */
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
    // A REJECTION IS NOT CACHED. The chunk this awaits is fetched over the dev server or read
    // off disk, and either can fail once — a memoised rejection turns that single failure into
    // a session with no syntax highlighting anywhere and no way back except a reload.
    //
    // The slot is cleared only if it still holds THIS attempt: a later caller may already have
    // started a successor, and clearing that one would make every block reload the grammars.
    const attempt: Promise<Highlighter> = pending.catch((error: unknown) => {
      if (promise === attempt) promise = null;
      throw error;
    });
    promise = attempt;
  }
  return promise;
}

// Both tables are keyed by a path the person or the model chose, so they are Maps
// rather than object literals: an object inherits `constructor`, `toString` and
// `__proto__` as live keys, and a file named any of those handed the caller a
// FUNCTION where a language tag belongs, which threw one frame later on
// `lang.toLowerCase()` and took the whole diff view down with it.

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

// Tags a model writes in a fence that are not the tag Shiki loaded them under.
// Separate from the extension table on purpose: `c++` and `c#` are things only a
// human types, and an extension table that accepted them would be answering a
// question nobody asked.
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

// Map a file path to a Shiki language tag by extension (or a bare basename like
// "Dockerfile" / "Makefile"). Returns "text" for anything unrecognized; pass the
// result through [resolveLang] before use so an un-bundled tag still degrades
// cleanly. Used by the diff view to highlight each file in its OWN language
// rather than assuming one.
export function langFromPath(path: string): string {
  const base = path.slice(path.lastIndexOf("/") + 1);
  const byName = LANG_BY_FILENAME.get(base);
  if (byName) return byName;
  const ext = base.slice(base.lastIndexOf(".") + 1).toLowerCase();
  return LANG_BY_EXTENSION.get(ext) ?? "text";
}

// Pick the closest loaded language for a tag — Shiki throws on unknown
// langs, so we degrade to plain "text" if the model emits something we
// don't bundle (e.g., `kdl`, `nix`).
export function resolveLang(highlighter: Highlighter, lang: string): string {
  // The loaded set is a couple of dozen entries and this runs once per file in a
  // diff, so scanning the array beats building a Set to throw away.
  const loaded = highlighter.getLoadedLanguages();
  if (loaded.includes(lang)) return lang;
  const aliased = LANG_BY_ALIAS.get(lang.toLowerCase());
  if (aliased && loaded.includes(aliased)) return aliased;
  return "text";
}
