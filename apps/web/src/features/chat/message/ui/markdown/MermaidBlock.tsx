import { iconGlyph } from "@/ui/icons/index";
import * as stylex from "@stylexjs/stylex";
import { useEffect, useId, useMemo, useState } from "react";
import { useDebouncedValue } from "@tanstack/react-pacer";
import { LightboxDialog } from "@flame/ui";
import { ShikiCodeBlock } from "@/ui/index";
import { reveal } from "@/ui/index";
import { IconButton } from "@flame/ui";
import { measureMermaidRender } from "@/lib/metrics";
import { useT } from "@/lib/i18n/index";
import { useCopyFeedback } from "@flame/ui";
import { cn } from "@/lib/classNames";
import { motion, radius, space, surface } from "@flame/ui/tokens.stylex";
import { artifactFrame } from "@/ui/atoms/artifact-frame";
import { MermaidViewer } from "@/features/chat/message/ui/markdown/MermaidViewer";
import { renderMermaid } from "@/features/chat/message/ui/markdown/mermaid";
const mb = stylex.create({
  frame: {
    position: "relative",
    isolation: "isolate",
    minHeight: "calc(var(--spacing) * 25)",
    width: "100%",
  },
  tools: {
    position: "absolute",
    top: space.s1,
    right: space.s1_5,
    zIndex: 1,
    display: "flex",
    gap: space.s1,
    transitionProperty: "opacity",
    transitionDuration: motion.fast,
    transitionTimingFunction: motion.easeState,
  },
  sourceText: { whiteSpace: "pre-wrap" },
  loading: {
    position: "relative",
    display: "grid",
    height: "calc(var(--spacing) * 60)",
    minHeight: "calc(var(--spacing) * 25)",
    width: "100%",
    placeItems: "center",
    overflow: "hidden",
  },
  stage: { overflowX: "auto", padding: space.s4 },
  viewerShell: {
    width: "min(92vw, 1400px)",
    height: "min(85dvh, 900px)",
    maxWidth: "none",
    maxHeight: "none",
    padding: 0,
    overflow: "hidden",
  },
  pulse: {
    height: space.s8,
    width: space.s8,
    borderRadius: radius.card,
    backgroundColor: surface.surface3,
    animation: motion.breathe,
  },
});

interface Props {
  code: string;
}

type MermaidRenderResult = { status: "loading" | "error" } | { status: "rendered"; svg: string };

interface SettledMermaidRender {
  code: string;
  result: MermaidRenderResult;
}

export function MermaidBlock({ code }: Props) {
  const t = useT();
  const fencedCode = useMemo(() => `\`\`\`mermaid\n${code}\n\`\`\``, [code]);
  const { copied, copy } = useCopyFeedback(fencedCode);
  const idPrefix = `mermaid-${useId().replace(/[^A-Za-z0-9_-]/g, "_")}`;
  const [debouncedCode] = useDebouncedValue(code, { wait: 300 });
  const isSettling = code !== debouncedCode;

  const [settledRender, setSettledRender] = useState<SettledMermaidRender | null>(null);
  useEffect(() => {
    if (isSettling) return;
    let cancelled = false;
    void Promise.resolve().then(async () => {
      let result: MermaidRenderResult;
      try {
        const start = performance.now();
        const svg = await renderMermaid(debouncedCode, idPrefix);
        measureMermaidRender(performance.now() - start);
        result = { status: "rendered", svg };
      } catch {
        result = { status: "error" };
      }
      if (!cancelled) setSettledRender({ code: debouncedCode, result });
    });
    return () => {
      cancelled = true;
    };
  }, [debouncedCode, isSettling, idPrefix]);
  const current: MermaidRenderResult =
    settledRender?.code === debouncedCode && !isSettling
      ? settledRender.result
      : { status: "loading" };
  const [lastSvg, setLastSvg] = useState<string | null>(null);
  const currentSvg = current.status === "rendered" ? current.svg : null;
  if (currentSvg !== null && currentSvg !== lastSvg) setLastSvg(currentSvg);
  const rendered: MermaidRenderResult =
    current.status === "loading" && lastSvg !== null
      ? { status: "rendered", svg: lastSvg }
      : current;

  const [zoomed, setZoomed] = useState(false);

  if (rendered.status === "rendered") {
    const svg = rendered.svg;
    return (
      <div
        {...stylex.props(reveal.host, artifactFrame.frame, mb.frame)}
        data-markdown-copy="code-block"
        data-markdown-copy-text={fencedCode}
        data-wide-markdown-block="true"
      >
        <div
          role="img"
          aria-label={t("markdown.diagram")}
          tabIndex={-1}
          dir="ltr"
          data-slot="mermaid-stage"
          className={stylex.props(mb.stage).className}
          dangerouslySetInnerHTML={{ __html: svg }}
        />
        <div
          data-reveal="hover"
          {...stylex.props(mb.tools, reveal.shown)}
          data-markdown-copy="exclude"
        >
          <LightboxDialog
            open={zoomed}
            onOpenChange={setZoomed}
            title={t("markdown.diagram")}
            styles={mb.viewerShell}
            trigger={
              <IconButton
                icon={iconGlyph("maximize")}
                size="xs"
                quiet
                aria-label={t("message.mermaid.enlarge")}
                aria-haspopup="dialog"
              />
            }
          >
            <MermaidViewer svg={svg} onClose={() => setZoomed(false)} />
          </LightboxDialog>
          <IconButton
            icon={iconGlyph(copied ? "check" : "copy")}
            size="xs"
            quiet
            onClick={() => void copy()}
            aria-label={t(copied ? "message.mermaid.copied" : "message.mermaid.copy")}
            tone={copied ? "success" : undefined}
          />
        </div>
        <span className="sr-only">{t("message.mermaid.source")}</span>
        <pre className={cn("sr-only", stylex.props(mb.sourceText).className)}>{code}</pre>
      </div>
    );
  }

  if (rendered.status === "loading") {
    return (
      <div
        role="status"
        aria-label={t("message.mermaid.loading")}
        {...stylex.props(artifactFrame.frame, mb.loading)}
      >
        <span aria-hidden="true" {...stylex.props(mb.pulse)} />
      </div>
    );
  }

  return <ShikiCodeBlock lang="mermaid" code={code} />;
}
