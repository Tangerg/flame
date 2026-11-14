import { basename, fileExtension } from "@/lib/path";
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

export function canonicalLanguage(lang: string): string {
  return LANG_BY_ALIAS.get(lang.toLowerCase()) ?? lang.toLowerCase();
}
