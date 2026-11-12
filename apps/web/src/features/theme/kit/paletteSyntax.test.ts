import { afterAll, beforeAll, describe, expect, it, vi } from "vitest";
import {
  getTokenStyleObject,
  type HighlighterCore,
  type ShikiTransformer,
  type ThemedToken,
} from "shiki/core";
import { disposeHighlighter, getHighlighter, highlightCode } from "@/lib/highlight/shiki";
import { shikiThemeName, syntaxColorToken } from "@/lib/highlight/shikiTheme";
import { paletteThemeContribution } from "@/features/theme/kit/palette";
import { contrastRatio } from "@/features/theme/kit/legibility";
import { type Scheme } from "@/lib/appearance";

beforeAll(async () => {
  await getHighlighter("typescript");
  await getHighlighter("markdown");
});
afterAll(disposeHighlighter);

const source = [
  "// Keep whitespace and escaped markup.",
  "export const retry = async (key: string) => {",
  '\tconst literal = "<script>throw new Error(1)</script>";',
  "",
  '  return key === "ArrowLeft" ? literal : undefined;',
  "};",
  "",
].join("\n");

function foregroundCharacters(root: HTMLElement) {
  const characters: { character: string; foreground: string }[] = [];
  const text = document.createTreeWalker(root.querySelector("code")!, NodeFilter.SHOW_TEXT);
  while (text.nextNode()) {
    let parent = text.currentNode.parentElement;
    let foreground: string | undefined;
    while (parent && !foreground) {
      foreground = parent.getAttribute("style")?.match(/(?:^|;)\s*color:\s*([^;]+)/i)?.[1];
      parent = parent.parentElement;
    }
    if (!foreground) throw new Error("syntax text has no foreground");
    for (const character of text.currentNode.textContent ?? "") {
      if (character.trim()) characters.push({ character, foreground: foreground.toLowerCase() });
    }
  }
  return characters;
}

// The tokenizer's per-line time budget makes separate passes disagree, so the oracle is the
// authored token stream of the very pass that renders, captured before flame:syntax-color runs.
function observeHighlight(highlighter: HighlighterCore, render: () => string) {
  const passes: ThemedToken[][][] = [];
  const observer: ShikiTransformer = {
    name: "test:authored-tokens",
    enforce: "pre",
    tokens(lines) {
      passes.push(lines.map((line) => line.map((token) => ({ ...token }))));
    },
  };
  const codeToHtml = highlighter.codeToHtml.bind(highlighter);
  const spy = vi
    .spyOn(highlighter, "codeToHtml")
    .mockImplementation((code, options) =>
      codeToHtml(code, { ...options, transformers: [observer, ...(options.transformers ?? [])] }),
    );
  let html: string;
  try {
    html = render();
  } finally {
    spy.mockRestore();
  }
  expect(passes).toHaveLength(1);
  const root = document.createElement("div");
  root.innerHTML = html;
  return { root, authored: passes[0]! };
}

function expectPaletteForegrounds(scheme: Scheme, root: HTMLElement, authored: ThemedToken[][]) {
  expect(root.querySelector("code")?.textContent).toBe(source);
  expect(root.querySelector("script")).toBeNull();
  expect(authored.map((line) => line.map((token) => token.content).join("")).join("\n")).toBe(
    source,
  );
  const expected = authored
    .flat()
    .flatMap((token) =>
      [...token.content]
        .filter((character) => character.trim())
        .map((character) => ({ character, token })),
    );
  const painted = foregroundCharacters(root);
  expect(painted.length).toBeGreaterThan(1);
  expect(painted).toHaveLength(expected.length);

  const palette = paletteThemeContribution(
    {
      id: "reading",
      label: "Reading",
      scheme,
      palette:
        scheme === "dark"
          ? { background: "#505967", foreground: "#f0f4fb" }
          : { background: "#afa996", foreground: "#111111" },
    },
    "#0485f7",
    25,
  ).tokens!;
  for (const [index, paint] of painted.entries()) {
    const { character, token } = expected[index]!;
    expect(token.bgColor).toBeUndefined();
    const rawColor = token.color?.toLowerCase() ?? "";
    expect(rawColor).toMatch(/^#[\da-f]+$/);
    const property = syntaxColorToken(rawColor);
    expect(paint.character).toBe(character);
    expect(paint.foreground).toBe(`var(--${property}, ${rawColor})`);
    expect(contrastRatio(palette[property]!, palette["color-bg"]!)).toBeGreaterThanOrEqual(4.5);
  }
}

describe.each(["light", "dark"] as const)("%s syntax foregrounds", (scheme: Scheme) => {
  it("retains source, escaping and authored syntax roles through palette references", async () => {
    const highlighter = await getHighlighter("typescript");
    const options = { lang: "typescript", theme: shikiThemeName(scheme) };
    const { root, authored } = observeHighlight(highlighter, () =>
      highlightCode(highlighter, source, options),
    );
    expectPaletteForegrounds(scheme, root, authored);
  });

  it("maps the foreground a time-limited tokenizer pass authors", async () => {
    const highlighter = await getHighlighter("typescript");
    const options = { lang: "typescript", theme: shikiThemeName(scheme) };
    let clock = Date.now();
    const { root, authored } = observeHighlight(highlighter, () => {
      const now = vi.spyOn(Date, "now").mockImplementation(() => (clock += 1000));
      try {
        return highlightCode(highlighter, source, options);
      } finally {
        now.mockRestore();
      }
    });
    expect(new Set(authored.flat().map((token) => token.color)).size).toBe(1);
    expectPaletteForegrounds(scheme, root, authored);
  });

  it("keeps syntax emphasis while replacing its foreground", async () => {
    const highlighter = await getHighlighter("markdown");
    const options = { lang: "markdown", theme: shikiThemeName(scheme) };
    const code = "*emphasis* and **strong**";
    const { root, authored } = observeHighlight(highlighter, () =>
      highlightCode(highlighter, code, options),
    );
    const emphasis = authored.flat().map((token) => {
      const style = getTokenStyleObject(token);
      const span = document.createElement("span");
      span.style.fontStyle = style["font-style"] ?? "";
      span.style.fontWeight = style["font-weight"] ?? "";
      return [token.content, span.style.fontStyle, span.style.fontWeight];
    });
    expect(emphasis.some(([, fontStyle, fontWeight]) => fontStyle || fontWeight)).toBe(true);
    const painted = [...root.querySelectorAll<HTMLElement>("span[style]")];
    expect(
      painted.map((span) => [span.textContent, span.style.fontStyle, span.style.fontWeight]),
    ).toEqual(emphasis);
  });
});
