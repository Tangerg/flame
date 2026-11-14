import type { LanguageRegistration } from "shiki/core";
import { canonicalLanguage } from "./languages";

const LANGUAGES = new Map([
  ["typescript", () => import(/* @flame-lazy:syntax-highlighting */ "shiki/langs/typescript.mjs")],
  ["javascript", () => import(/* @flame-lazy:syntax-highlighting */ "shiki/langs/javascript.mjs")],
  ["tsx", () => import(/* @flame-lazy:syntax-highlighting */ "shiki/langs/tsx.mjs")],
  ["jsx", () => import(/* @flame-lazy:syntax-highlighting */ "shiki/langs/jsx.mjs")],
  ["python", () => import(/* @flame-lazy:syntax-highlighting */ "shiki/langs/python.mjs")],
  ["go", () => import(/* @flame-lazy:syntax-highlighting */ "shiki/langs/go.mjs")],
  ["rust", () => import(/* @flame-lazy:syntax-highlighting */ "shiki/langs/rust.mjs")],
  ["java", () => import(/* @flame-lazy:syntax-highlighting */ "shiki/langs/java.mjs")],
  ["c", () => import(/* @flame-lazy:syntax-highlighting */ "shiki/langs/c.mjs")],
  ["cpp", () => import(/* @flame-lazy:syntax-highlighting */ "shiki/langs/cpp.mjs")],
  ["csharp", () => import(/* @flame-lazy:syntax-highlighting */ "shiki/langs/csharp.mjs")],
  ["ruby", () => import(/* @flame-lazy:syntax-highlighting */ "shiki/langs/ruby.mjs")],
  ["php", () => import(/* @flame-lazy:syntax-highlighting */ "shiki/langs/php.mjs")],
  ["swift", () => import(/* @flame-lazy:syntax-highlighting */ "shiki/langs/swift.mjs")],
  ["kotlin", () => import(/* @flame-lazy:syntax-highlighting */ "shiki/langs/kotlin.mjs")],
  ["bash", () => import(/* @flame-lazy:syntax-highlighting */ "shiki/langs/bash.mjs")],
  ["json", () => import(/* @flame-lazy:syntax-highlighting */ "shiki/langs/json.mjs")],
  ["yaml", () => import(/* @flame-lazy:syntax-highlighting */ "shiki/langs/yaml.mjs")],
  ["toml", () => import(/* @flame-lazy:syntax-highlighting */ "shiki/langs/toml.mjs")],
  ["html", () => import(/* @flame-lazy:syntax-highlighting */ "shiki/langs/html.mjs")],
  ["css", () => import(/* @flame-lazy:syntax-highlighting */ "shiki/langs/css.mjs")],
  ["scss", () => import(/* @flame-lazy:syntax-highlighting */ "shiki/langs/scss.mjs")],
  ["markdown", () => import(/* @flame-lazy:syntax-highlighting */ "shiki/langs/markdown.mjs")],
  ["sql", () => import(/* @flame-lazy:syntax-highlighting */ "shiki/langs/sql.mjs")],
  ["diff", () => import(/* @flame-lazy:syntax-highlighting */ "shiki/langs/diff.mjs")],
  ["dockerfile", () => import(/* @flame-lazy:syntax-highlighting */ "shiki/langs/dockerfile.mjs")],
  ["graphql", () => import(/* @flame-lazy:syntax-highlighting */ "shiki/langs/graphql.mjs")],
  ["xml", () => import(/* @flame-lazy:syntax-highlighting */ "shiki/langs/xml.mjs")],
  ["regexp", () => import(/* @flame-lazy:syntax-highlighting */ "shiki/langs/regexp.mjs")],
  ["glsl", () => import(/* @flame-lazy:syntax-highlighting */ "shiki/langs/glsl.mjs")],
  ["cpp-macro", () => import(/* @flame-lazy:syntax-highlighting */ "shiki/langs/cpp-macro.mjs")],
  ["haml", () => import(/* @flame-lazy:syntax-highlighting */ "shiki/langs/haml.mjs")],
  ["lua", () => import(/* @flame-lazy:syntax-highlighting */ "shiki/langs/lua.mjs")],
]);

export async function loadHighlightLanguage(lang: string): Promise<LanguageRegistration[]> {
  const load = LANGUAGES.get(canonicalLanguage(lang));
  return load ? (await load()).default : [];
}
