import { afterAll, beforeAll, describe, expect, it } from "vitest";
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

describe.each(["light", "dark"] as const)("%s syntax foregrounds", (scheme: Scheme) => {
  it("retains source, escaping and authored syntax roles through palette references", async () => {
    const highlighter = await getHighlighter("typescript");
    const options = { lang: "typescript", theme: shikiThemeName(scheme) };
    const original = document.createElement("div");
    original.innerHTML = highlighter.codeToHtml(source, options);
    const themed = document.createElement("div");
    themed.innerHTML = highlightCode(highlighter, source, options);

    expect(themed.querySelector("code")?.textContent).toBe(source);
    expect(themed.querySelector("script")).toBeNull();
    const spans = [...themed.querySelectorAll("span[style]")];
    const authored = [...original.querySelectorAll("span[style]")];
    expect(spans.length).toBeGreaterThan(1);
    expect(spans).toHaveLength(authored.length);

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
    for (const [index, span] of spans.entries()) {
      const rawColor = authored[index]!.getAttribute("style")!.match(/color:\s*(#[\da-f]+)/i)![1]!;
      const property = syntaxColorToken(rawColor);
      expect(span.textContent).toBe(authored[index]!.textContent);
      expect(span.getAttribute("style")).toContain(`var(--${property}, ${rawColor})`);
      expect(contrastRatio(palette[property]!, palette["color-bg"]!)).toBeGreaterThanOrEqual(4.5);
    }
  });

  it("keeps syntax emphasis while replacing its foreground", async () => {
    const highlighter = await getHighlighter("markdown");
    const options = { lang: "markdown", theme: shikiThemeName(scheme) };
    const code = "*emphasis* and **strong**";
    const original = document.createElement("div");
    original.innerHTML = highlighter.codeToHtml(code, options);
    const themed = document.createElement("div");
    themed.innerHTML = highlightCode(highlighter, code, options);
    const authored = [...original.querySelectorAll<HTMLElement>("span[style]")];
    const painted = [...themed.querySelectorAll<HTMLElement>("span[style]")];
    expect(authored.some((span) => span.style.fontStyle || span.style.fontWeight)).toBe(true);
    expect(
      painted.map((span) => [span.textContent, span.style.fontStyle, span.style.fontWeight]),
    ).toEqual(
      authored.map((span) => [span.textContent, span.style.fontStyle, span.style.fontWeight]),
    );
  });
});
