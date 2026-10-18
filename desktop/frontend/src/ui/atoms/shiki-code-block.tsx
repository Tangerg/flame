import * as stylex from "@stylexjs/stylex";
import { useEffect, useState, type ReactNode } from "react";
import { useDebouncedValue } from "@tanstack/react-pacer";
import { CodeCaption, useCodeWrap } from "@flame/ui";
import { measureShikiHighlight } from "@/lib/metrics";
import { getHighlighter, reportHighlightFailure, resolveLang } from "@/lib/highlight/shiki";
import { getCachedHighlight, setCachedHighlight } from "@/lib/highlight/shikiCache";
import { useShikiTheme } from "@/lib/highlight/useCodeHighlight";
import { cn } from "@/lib/classNames";
import { space, type } from "@flame/ui/tokens.stylex";
import { artifactFrame } from "./artifact-frame";
import { reveal } from "./reveal";
import { useCodeActionLabels } from "./codeActionLabels";
import type { ShikiTransformer } from "shiki";

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
  scrollLabel?: string;
  copyable?: boolean;
}

interface HighlightedCode {
  lang: string;
  theme: string;
  code: string;
  html: string;
}

const styles = stylex.create({
  block: {
    overflow: "hidden",
    fontFamily: "var(--font-mono)",
  },
  embedded: { marginBlock: 0 },
  previewBody: {
    display: "grid",
    maxHeight: "calc(15lh + 24px)",
    placeItems: "center",
    overflow: "auto",
    padding: space.s3,
  },
  bounded: {
    maxHeight: "calc(var(--spacing) * 60)",
    overflow: "auto",
    overscrollBehavior: "contain",
  },
  fallback: { margin: 0 },
});

export function ShikiCodeBlock({
  lang,
  code,
  preview,
  previewLabel,
  scrollLabel,
  copyable = true,
}: Props) {
  const labels = useCodeActionLabels();
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
  const wrapCode = useCodeWrap();

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
  const source = showHighlighted ? (
    <div
      className="shiki-body"
      data-wrap={wrapCode}
      ref={
        scrollLabel === undefined
          ? undefined
          : (node) => {
              // The stable scroll region owns focus across generated source replacements.
              const pre = node?.querySelector("pre");
              if (pre) pre.tabIndex = -1;
            }
      }
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
  );
  const block = stylex.props(
    artifactFrame.frame,
    styles.block,
    scrollLabel !== undefined && styles.embedded,
    type.code,
    isPreview && reveal.host,
  );
  return (
    <div
      dir="ltr"
      data-variant={isPreview ? "preview" : "code"}
      data-markdown-copy="code-block"
      data-markdown-copy-text={code}
      {...block}
      className={cn(block.className, "shiki-block")}
    >
      <CodeCaption
        data-markdown-copy="exclude"
        language={lang}
        code={code}
        labels={labels}
        wrappable={!isPreview}
        copyable={copyable}
        copyReveal={
          isPreview
            ? { "data-reveal": "hover", className: stylex.props(reveal.shown).className ?? "" }
            : undefined
        }
      />
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
      ) : scrollLabel === undefined ? (
        source
      ) : (
        <div
          data-slot="shiki-scroll"
          data-focus-inset=""
          role="region"
          aria-label={scrollLabel}
          // oxlint-disable-next-line jsx-a11y/no-noninteractive-tabindex
          tabIndex={0}
          {...stylex.props(styles.bounded)}
        >
          {source}
        </div>
      )}
    </div>
  );
}
