import { type HighlighterCore } from "shiki/core";
import { basename, fileExtension } from "@/lib/path";
const LANGUAGES = new Map([
  ["typescript", () => import("shiki/langs/typescript.mjs")],
  ["javascript", () => import("shiki/langs/javascript.mjs")],
  ["tsx", () => import("shiki/langs/tsx.mjs")],
  ["jsx", () => import("shiki/langs/jsx.mjs")],
  ["python", () => import("shiki/langs/python.mjs")],
  ["go", () => import("shiki/langs/go.mjs")],
  ["rust", () => import("shiki/langs/rust.mjs")],
  ["java", () => import("shiki/langs/java.mjs")],
  ["c", () => import("shiki/langs/c.mjs")],
  ["cpp", () => import("shiki/langs/cpp.mjs")],
  ["csharp", () => import("shiki/langs/csharp.mjs")],
  ["ruby", () => import("shiki/langs/ruby.mjs")],
  ["php", () => import("shiki/langs/php.mjs")],
  ["swift", () => import("shiki/langs/swift.mjs")],
  ["kotlin", () => import("shiki/langs/kotlin.mjs")],
  ["bash", () => import("shiki/langs/bash.mjs")],
  ["json", () => import("shiki/langs/json.mjs")],
  ["yaml", () => import("shiki/langs/yaml.mjs")],
  ["toml", () => import("shiki/langs/toml.mjs")],
  ["html", () => import("shiki/langs/html.mjs")],
  ["css", () => import("shiki/langs/css.mjs")],
  ["scss", () => import("shiki/langs/scss.mjs")],
  ["markdown", () => import("shiki/langs/markdown.mjs")],
  ["sql", () => import("shiki/langs/sql.mjs")],
  ["diff", () => import("shiki/langs/diff.mjs")],
  ["dockerfile", () => import("shiki/langs/dockerfile.mjs")],
  ["graphql", () => import("shiki/langs/graphql.mjs")],
  ["xml", () => import("shiki/langs/xml.mjs")],
  ["regexp", () => import("shiki/langs/regexp.mjs")],
  ["glsl", () => import("shiki/langs/glsl.mjs")],
  ["cpp-macro", () => import("shiki/langs/cpp-macro.mjs")],
  ["haml", () => import("shiki/langs/haml.mjs")],
  ["lua", () => import("shiki/langs/lua.mjs")],
]);

const reported = new Set<string>();

export function reportHighlightFailure(cause: string, error: unknown): void {
  if (reported.has(cause)) return;
  reported.add(cause);
  console.error(`[highlight] ${cause}:`, error);
}

let promise: Promise<HighlighterCore> | null = null;

export async function getHighlighter(lang = "text"): Promise<HighlighterCore> {
  if (promise === null) {
    const pending = Promise.all([
      import("shiki/core"),
      import("shiki/engine/javascript"),
      import("shiki/themes/github-dark.mjs"),
      import("shiki/themes/github-light-high-contrast.mjs"),
    ]).then(([{ createHighlighterCore }, { createJavaScriptRegexEngine }, dark, light]) =>
      createHighlighterCore({
        engine: createJavaScriptRegexEngine({ target: "ES2018" }),
        themes: [dark.default, light.default],
        langs: [],
      }),
    );
    const attempt = pending.catch((error: unknown) => {
      if (promise === attempt) promise = null;
      throw error;
    });
    promise = attempt;
  }
  const highlighter = await promise;
  const name = LANG_BY_ALIAS.get(lang.toLowerCase()) ?? lang.toLowerCase();
  const load = LANGUAGES.get(name);
  if (load) await highlighter.loadLanguage(load);
  return highlighter;
}

export async function disposeHighlighter(): Promise<void> {
  const pending = promise;
  promise = null;
  if (pending) (await pending).dispose();
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
  ["cts", "typescript"],
  ["mts", "typescript"],
  ["cjs", "javascript"],
  ["mjs", "javascript"],
  ["kt", "kotlin"],
  ["kts", "kotlin"],
  ["md", "markdown"],
  ["gql", "graphql"],
  ["regex", "regexp"],
  ["shellscript", "bash"],
  ["ts", "typescript"],
  ["js", "javascript"],
  ["py", "python"],
  ["rb", "ruby"],
  ["rs", "rust"],
  ["sh", "bash"],
  ["shell", "bash"],
  ["zsh", "bash"],
  ["yml", "yaml"],
  ["dockerfile", "dockerfile"],
  ["docker", "dockerfile"],
  ["c++", "cpp"],
  ["c#", "csharp"],
  ["cs", "csharp"],
]);

export function langFromPath(path: string): string {
  const base = basename(path);
  const byName = LANG_BY_FILENAME.get(base);
  if (byName) return byName;
  return LANG_BY_EXTENSION.get(fileExtension(path)) ?? "text";
}

export function resolveLang(highlighter: HighlighterCore, lang: string): string {
  const loaded = highlighter.getLoadedLanguages();
  if (loaded.includes(lang)) return lang;
  const aliased = LANG_BY_ALIAS.get(lang.toLowerCase()) ?? lang.toLowerCase();
  if (aliased && loaded.includes(aliased)) return aliased;
  return "text";
}
