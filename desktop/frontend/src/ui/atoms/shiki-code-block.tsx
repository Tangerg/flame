import { useEffect, useState, type ReactNode } from "react";
import { useDebouncedValue } from "@tanstack/react-pacer";
import { useCopyFeedback } from "@/lib/useCopyFeedback";
import { measureShikiHighlight } from "@/lib/metrics";
import { getHighlighter, reportHighlightFailure, resolveLang } from "@/lib/highlight/shiki";
import { getCachedHighlight, setCachedHighlight } from "@/lib/highlight/shikiCache";
import { useShikiTheme } from "@/lib/highlight/useCodeHighlight";
import { cn } from "@/lib/classNames";
import { toggleCodeWrapPreference, useCodeWrapPreference } from "./codeWrapPreference";
import { useT } from "@/lib/i18n";
import { IconButton } from "./icon-button";

interface Props {
  lang: string;
  code: string;
  file?: string;
  preview?: ReactNode;
  previewLabel?: string;
}

interface HighlightedCode {
  lang: string;
  theme: string;
  code: string;
  html: string;
}

export function ShikiCodeBlock({ lang, code, file, preview, previewLabel }: Props) {
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

  return (
    <div
      dir="ltr"
      data-variant={isPreview ? "preview" : "code"}
      data-markdown-copy="code-block"
      data-markdown-copy-text={code}
      className={cn(
        "shiki-block group/code my-3.5 overflow-hidden font-mono text-code",
        isPreview
          ? "group/code-snippet rounded-lg border-[0.5px] border-field bg-transparent"
          : "rounded-lg bg-sunken",
      )}
    >
      <div
        data-markdown-copy="exclude"
        className="flex items-center gap-2 bg-transparent px-2 py-1 font-sans text-ui-md"
      >
        <span
          className={cn(
            "shrink-0 text-fg-muted",
            isPreview
              ? "font-sans text-ui-md tracking-normal"
              : "font-sans text-ui-md font-normal tracking-normal normal-case",
          )}
        >
          {lang || "text"}
        </span>
        {file && (
          <span className="min-w-0 flex-1 truncate font-sans text-ui-md text-fg-muted">{file}</span>
        )}
        <span className="min-w-1 flex-1" />
        {!isPreview && (
          <IconButton
            icon={wrapCode ? "wrap-text" : "unfold-horizontal"}
            size="xs"
            active={wrapCode}
            aria-pressed={wrapCode}
            onClick={toggleCodeWrapPreference}
            title={t(wrapCode ? "message.code.wrap.disable" : "message.code.wrap.enable")}
            className="text-fg-faint hover:bg-hover hover:text-fg"
          />
        )}
        <IconButton
          data-reveal={isPreview ? "hover" : undefined}
          icon={copied ? "check" : "copy"}
          size="xs"
          onClick={() => void copy()}
          title={copied ? t("message.code.copied") : t("message.code.copy")}
          className={cn(
            copied ? "text-success" : "text-fg-faint hover:bg-hover hover:text-fg",
            isPreview &&
              "pointer-events-none opacity-0 transition-opacity group-hover/code-snippet:pointer-events-auto group-hover/code-snippet:opacity-100 focus-visible:pointer-events-auto focus-visible:opacity-100",
          )}
        />
      </div>
      {isPreview ? (
        <div
          data-slot="shiki-preview-body"
          data-focus-inset=""
          className="grid max-h-[calc(15lh+16px)] place-items-center overflow-auto p-2"
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
        <pre className="shiki-body shiki-fallback m-0" data-wrap={wrapCode}>
          {code}
        </pre>
      )}
    </div>
  );
}
