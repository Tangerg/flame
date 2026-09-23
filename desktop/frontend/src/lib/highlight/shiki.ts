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

const reported = new Set<string>();

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
    const attempt: Promise<Highlighter> = pending.catch((error: unknown) => {
      if (promise === attempt) promise = null;
      throw error;
    });
    promise = attempt;
  }
  return promise;
}

const LANG_BY_FILENAME = new Map([
  ["Dockerfile", "dockerfile"],
  ["Makefile", "bash"],
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

export function langFromPath(path: string): string {
  const base = path.slice(path.lastIndexOf("/") + 1);
  const byName = LANG_BY_FILENAME.get(base);
  if (byName) return byName;
  const ext = base.slice(base.lastIndexOf(".") + 1).toLowerCase();
  return LANG_BY_EXTENSION.get(ext) ?? "text";
}

export function resolveLang(highlighter: Highlighter, lang: string): string {
  const loaded = highlighter.getLoadedLanguages();
  if (loaded.includes(lang)) return lang;
  const aliased = LANG_BY_ALIAS.get(lang.toLowerCase());
  if (aliased && loaded.includes(aliased)) return aliased;
  return "text";
}
