import { type HighlighterCore, type ShikiTransformer, type LanguageRegistration } from "shiki/core";
import { canonicalLanguage } from "./languages";
import { SHIKI_THEME } from "@/lib/highlight/shikiTheme";
let promise: Promise<HighlighterCore> | null = null;

export async function getHighlighter(
  grammars: LanguageRegistration[] = [],
): Promise<HighlighterCore> {
  if (promise === null) {
    const pending = Promise.all([import("shiki/core"), import("shiki/engine/javascript")]).then(
      ([{ createHighlighterCore }, { createJavaScriptRegexEngine }]) =>
        createHighlighterCore({
          engine: createJavaScriptRegexEngine({ target: "ES2018" }),
          themes: [SHIKI_THEME.dark, SHIKI_THEME.light],
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
  if (grammars.length) await highlighter.loadLanguage(...grammars);
  return highlighter;
}

// Tokens paint through palette-owned properties, so a palette change repaints retained and cached
// HTML without re-tokenizing; the authored color remains the fallback. A token carrying its own
// background keeps the pairing its theme authored.
const SYNTAX_COLOR: ShikiTransformer = {
  name: "flame:syntax-color",
  tokens: (lines) =>
    lines.map((line) =>
      line.map((token) =>
        token.color?.startsWith("#") && !token.bgColor
          ? {
              ...token,
              // The literal read prefix lets the token guard detect removal of this consumer.
              color: `var(--color-code-${token.color.slice(1).toLowerCase()}, ${token.color})`,
            }
          : token,
      ),
    ),
};

export function highlightCode(
  highlighter: HighlighterCore,
  code: string,
  options: { lang: string; theme: string },
): string {
  return highlighter.codeToHtml(code, { ...options, transformers: [SYNTAX_COLOR] });
}

export async function renderHighlight(
  code: string,
  lang: string,
  theme: string,
  grammars: LanguageRegistration[],
) {
  const highlighter = await getHighlighter(grammars);
  const resolved = resolveLang(highlighter, lang);
  const start = performance.now();
  const html = highlightCode(highlighter, code, { lang: resolved, theme });
  return { html, lang: resolved, duration: performance.now() - start };
}

export async function disposeHighlighter(): Promise<void> {
  const pending = promise;
  promise = null;
  if (pending) (await pending).dispose();
}

export function resolveLang(highlighter: HighlighterCore, lang: string): string {
  const loaded = highlighter.getLoadedLanguages();
  if (loaded.includes(lang)) return lang;
  const aliased = canonicalLanguage(lang);
  if (aliased && loaded.includes(aliased)) return aliased;
  return "text";
}
