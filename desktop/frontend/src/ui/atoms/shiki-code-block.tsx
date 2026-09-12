import * as stylex from "@stylexjs/stylex";
import { useEffect, useState, type ReactNode } from "react";
import { useDebouncedValue } from "@tanstack/react-pacer";
import { useCopyFeedback } from "@/lib/useCopyFeedback";
import { measureShikiHighlight } from "@/lib/metrics";
import { getHighlighter, reportHighlightFailure, resolveLang } from "@/lib/highlight/shiki";
import { getCachedHighlight, setCachedHighlight } from "@/lib/highlight/shikiCache";
import { useShikiTheme } from "@/lib/highlight/useCodeHighlight";
import { cn } from "@/lib/classNames";
import { color, radius, space, surface, type, weight } from "@/styles/tokens.stylex";
import { reveal } from "./reveal";
import { toggleCodeWrapPreference, useCodeWrapPreference } from "./codeWrapPreference";
import { useT } from "@/lib/i18n";
import { IconButton } from "./icon-button";
import type { ShikiTransformer } from "shiki";

// Shiki gives the `<pre>` it emits a `tabindex`, so the scrollable code takes keyboard focus like
// any other scroll region. The rounded block wrapping it clips, which leaves the global ring with
// nowhere to draw; `data-focus-inset` is how a clipped control asks for it on the inside. Only
// this call site keeps that `<pre>` — the diff and file views strip the wrapper away.
const FOCUS_INSET: ShikiTransformer[] = [
  {
    pre(node) {
      node.properties["data-focus-inset"] = "";
    },
  },
];

interface Props {
  lang: string;
  code: string;
  preview?: ReactNode;
  previewLabel?: string;
}

interface HighlightedCode {
  lang: string;
  theme: string;
  code: string;
  html: string;
}

const styles = stylex.create({
  block: {
    marginBlock: "calc(var(--spacing) * 3.5)",
    overflow: "hidden",
    borderRadius: radius.lg,
    fontFamily: "var(--font-mono)",
  },
  framed: {
    borderWidth: "0.5px",
    borderStyle: "solid",
    borderColor: surface.field,
    backgroundColor: "transparent",
  },
  recessed: { backgroundColor: surface.sunken },
  caption: {
    display: "flex",
    alignItems: "center",
    gap: space.s2,
    backgroundColor: "transparent",
    paddingInline: space.s2,
    paddingBlock: space.s1,
    fontFamily: "var(--font-sans)",
  },
  lang: { flexShrink: 0, color: color.fgMuted, fontFamily: "var(--font-sans)" },
  langPlain: {
    letterSpacing: "var(--tracking-none)",
    textTransform: "none",
    fontWeight: weight.regular,
  },
  spacer: { minWidth: space.s1, flex: 1 },
  previewBody: {
    display: "grid",
    maxHeight: "calc(15lh + 16px)",
    placeItems: "center",
    overflow: "auto",
    padding: space.s2,
  },
  fallback: { margin: 0 },
});

export function ShikiCodeBlock({ lang, code, preview, previewLabel }: Props) {
  const t = useT();
  const shikiTheme = useShikiTheme();
  const isPreview = preview !== undefined;

  const [debouncedCode] = useDebouncedValue(code, { wait: 120 });
  const isSettling = code !== debouncedCode;

  const cachedHtml = getCachedHighlight(lang, shikiTheme, debouncedCode);
  const [highlighted, setHighlighted] = useState<HighlightedCode | null>(() =>
    cachedHtml === undefined
      ? null
      : { lang, theme: shikiTheme, code: debouncedCode, html: cachedHtml },
  );
  const html =
    cachedHtml ??
    (highlighted?.lang === lang &&
    highlighted.theme === shikiTheme &&
    highlighted.code === debouncedCode
      ? highlighted.html
      : null);
  const wrapCode = useCodeWrapPreference();
  const { copied, copy } = useCopyFeedback(code);

  useEffect(() => {
    if (cachedHtml !== undefined) return;

    let cancelled = false;
    getHighlighter()
      .then((h) => {
        if (cancelled) return;
        try {
          const resolvedLang = resolveLang(h, lang);
          const start = performance.now();
          const out = h.codeToHtml(debouncedCode, {
            lang: resolvedLang,
            theme: shikiTheme,
            transformers: FOCUS_INSET,
          });
          measureShikiHighlight(performance.now() - start, resolvedLang);
          setCachedHighlight(lang, shikiTheme, debouncedCode, out);
          setHighlighted({ lang, theme: shikiTheme, code: debouncedCode, html: out });
        } catch (error) {
          // One grammar failing leaves this block plain and every other block alone, so the
          // report is keyed by language: a file type Shiki cannot parse says so once.
          reportHighlightFailure(`grammar ${lang}`, error);
        }
      })
      .catch((error: unknown) => reportHighlightFailure("highlighter unavailable", error));
    return () => {
      cancelled = true;
    };
  }, [cachedHtml, lang, debouncedCode, shikiTheme]);

  const showHighlighted = !isSettling && html !== null;

  const fallback = stylex.props(styles.fallback);
  const block = stylex.props(
    styles.block,
    type.code,
    isPreview ? [reveal.host, styles.framed] : styles.recessed,
  );
  return (
    <div
      dir="ltr"
      data-variant={isPreview ? "preview" : "code"}
      data-markdown-copy="code-block"
      data-markdown-copy-text={code}
      {...block}
      // `shiki-block` and `shiki-body` are the highlighter's own hooks: Shiki writes the token
      // spans, and `globals.css` styles them several levels down. They stay classes.
      className={cn(block.className, "shiki-block")}
    >
      <div data-markdown-copy="exclude" {...stylex.props(styles.caption, type.uiMd)}>
        {/* The language, spelled as the highlighter reports it: no capitalising, and the UI
            tracking off, because a token like `tsx` is machine text wearing a proportional face. */}
        <span {...stylex.props(styles.lang, type.uiMd, styles.langPlain)}>{lang || "text"}</span>
        <span {...stylex.props(styles.spacer)} />
        {!isPreview && (
          <IconButton
            icon={wrapCode ? "wrap-text" : "unfold-horizontal"}
            size="xs"
            active={wrapCode}
            aria-pressed={wrapCode}
            onClick={toggleCodeWrapPreference}
            title={t(wrapCode ? "message.code.wrap.disable" : "message.code.wrap.enable")}
          />
        )}
        <IconButton
          data-reveal={isPreview ? "hover" : undefined}
          icon={copied ? "check" : "copy"}
          size="xs"
          onClick={() => void copy()}
          tone={copied ? "success" : undefined}
          title={copied ? t("message.code.copied") : t("message.code.copy")}
          className={cn(isPreview && stylex.props(reveal.shown).className)}
        />
      </div>
      {isPreview ? (
        <div
          data-slot="shiki-preview-body"
          data-focus-inset=""
          {...stylex.props(styles.previewBody)}
          role="region"
          aria-label={previewLabel}
          // oxlint-disable-next-line jsx-a11y/no-noninteractive-tabindex
          tabIndex={0}
        >
          {preview}
        </div>
      ) : showHighlighted ? (
        <div
          className="shiki-body"
          data-wrap={wrapCode}
          dangerouslySetInnerHTML={{ __html: html! }}
        />
      ) : (
        <pre
          {...fallback}
          className={cn(fallback.className, "shiki-body shiki-fallback")}
          data-wrap={wrapCode}
        >
          {code}
        </pre>
      )}
    </div>
  );
}
