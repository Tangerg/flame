import { expect, test } from "./test";

const samples = {
  regexp: "^(?<name>[a-z]+)\\d+$",
  glsl: "void main() { gl_Position = vec4(0.0); }",
  "cpp-macro": "#define VALUE 1",
  haml: "%div.class\n  %span Hello",
  lua: 'local value = "hello"',
  typescript: "const value: string = '中文';",
  javascript: "const value = /<script>/u;",
  tsx: "const view = <div>{value}</div>;",
  jsx: "const view = <div>{value}</div>;",
  python: 'def greet(name):\n    return f"Hello {name}"',
  go: 'package main\nfunc main() { println("hello") }',
  rust: 'fn main() { println!("hello"); }',
  java: 'class Main { String value = "hello"; }',
  c: "#include <stdio.h>\nint main(void) { return 0; }",
  cpp: "#include <vector>\nstd::vector<int> values{1, 2};",
  csharp: 'class Main { string value = "hello"; }',
  ruby: 'def greet(name)\n  "Hello #{name}"\nend',
  php: '<?php echo "hello"; ?>',
  swift: 'let value: String = "hello"',
  kotlin: 'fun main() { println("hello") }',
  bash: 'printf "%s\\n" "$HOME"',
  json: '{"value": "hello"}',
  yaml: "value: hello\nitems:\n  - one",
  toml: '[table]\nvalue = "hello"',
  html: '<p title="hello">中文</p>',
  css: ".box { color: #fff; }",
  scss: "$color: #fff;\n.box { color: $color; }",
  markdown: "# Title\n\n**strong** and `code`",
  sql: "SELECT name FROM users WHERE id = 1;",
  diff: "@@ -1 +1 @@\n-old\n+new",
  dockerfile: "FROM alpine:3\nRUN echo hello",
  graphql: "query { user(id: 1) { name } }",
  xml: '<?xml version="1.0"?><root>hello</root>',
};

test("highlights each supported grammar in both themes and disposes its engine", async ({
  page,
}) => {
  await page.goto("/visual/?fixture=agent&state=empty", { waitUntil: "domcontentloaded" });
  await page.locator("html[data-visual-ready]").waitFor();
  const result = await page.evaluate(async (samples) => {
    const modulePath = "/src/lib/highlight/shiki.ts";
    const { getHighlighter, disposeHighlighter, resolveLang } = await import(modulePath);
    await disposeHighlighter();
    const first = await getHighlighter("text");
    const initialLanguages = first.getLoadedLanguages();
    const rendered: string[] = [];
    for (const [lang, code] of Object.entries(samples)) {
      const highlighter = await getHighlighter(lang);
      if (highlighter !== first) throw new Error("language created another engine");
      for (const theme of ["github-dark", "github-light-high-contrast"]) {
        const html = highlighter.codeToHtml(code, { lang: resolveLang(highlighter, lang), theme });
        const root = document.createElement("div");
        root.innerHTML = html;
        if (root.querySelector("code")?.textContent !== code || root.querySelector("script")) {
          throw new Error(`highlight changed the source: ${lang}`);
        }
        rendered.push(`${lang}:${theme}`);
      }
    }
    for (const alias of [
      "cts",
      "mts",
      "cjs",
      "mjs",
      "kt",
      "kts",
      "md",
      "gql",
      "regex",
      "shellscript",
    ]) {
      const highlighter = await getHighlighter(alias);
      if (resolveLang(highlighter, alias) === "text") throw new Error(`missing alias: ${alias}`);
    }
    await disposeHighlighter();
    const next = await getHighlighter("text");
    const replaced = next !== first;
    await disposeHighlighter();
    return { initialLanguages, rendered, replaced };
  }, samples);
  expect(result.initialLanguages).toEqual([]);
  expect(result.rendered).toHaveLength(Object.keys(samples).length * 2);
  expect(result.replaced).toBe(true);
});
