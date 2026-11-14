import { expect, test } from "./test";
import { expectLabReady } from "../readiness";

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

test("highlights every grammar in both themes through one real Worker and retires it", async ({
  page,
}) => {
  await page.goto("/lab/fixture.html?fixture=agent&state=empty", { waitUntil: "domcontentloaded" });
  await expectLabReady(page);
  const result = await page.evaluate(async (samples) => {
    const modulePath = "/src/lib/highlight/shiki.ts";
    const { requestHighlight, disposeHighlighter } = await import(modulePath);
    await disposeHighlighter();
    const nativeWorker = window.Worker;
    const workers: Worker[] = [];
    window.Worker = class extends nativeWorker {
      constructor(url: string | URL, options?: WorkerOptions) {
        super(url, options);
        workers.push(this);
      }
    };
    const rendered: string[] = [];
    const owner = Symbol("grammar acceptance");
    const signal = new AbortController().signal;
    try {
      for (const [lang, code] of Object.entries(samples)) {
        for (const theme of ["github-dark", "github-light-high-contrast"]) {
          const result = await requestHighlight(owner, { code, lang, theme }, signal);
          if (result.lang === "text") throw new Error(`missing grammar: ${lang}`);
          const root = document.createElement("div");
          root.innerHTML = result.html;
          if (root.querySelector("code")?.textContent !== code || root.querySelector("script"))
            throw new Error(`highlight changed the source: ${lang}`);
          rendered.push(`${lang}:${theme}`);
        }
      }
      for (const lang of [
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
        const result = await requestHighlight(
          owner,
          {
            code: "alias",
            lang,
            theme: "github-dark",
          },
          signal,
        );
        if (result.lang === "text") throw new Error(`missing alias: ${lang}`);
      }
      const fallback = await requestHighlight(
        owner,
        {
          code: "literal unknown source",
          lang: "not-a-known-language",
          theme: "github-dark",
        },
        signal,
      );
      if (fallback.lang !== "text") throw new Error("unknown grammar changed its fallback");
      if (workers.length !== 1) throw new Error("highlight readers created multiple workers");
      await disposeHighlighter();
      await requestHighlight(
        owner,
        { code: "new lifetime", lang: "text", theme: "github-dark" },
        signal,
      );
      return { rendered, workers: workers.length };
    } finally {
      await disposeHighlighter();
      window.Worker = nativeWorker;
    }
  }, samples);
  expect(result.rendered).toHaveLength(Object.keys(samples).length * 2);
  expect(result.workers).toBe(2);
});
