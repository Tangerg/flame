import * as stylex from "@stylexjs/stylex";
import { useEffect, useMemo, useState } from "react";
import { useDebouncedValue } from "@tanstack/react-pacer";
import { IconButton, LightboxDialog, ShikiCodeBlock, reveal } from "@/ui";
import { measureMermaidRender } from "@/lib/metrics";
import { useT } from "@/lib/i18n";
import { useTokenRevision } from "@/lib/appearance";
import { useCopyFeedback } from "@/lib/useCopyFeedback";
import { cn } from "@/lib/classNames";
import { radius, space, surface } from "@/styles/tokens.stylex";

const mb = stylex.create({
  // Holds the diagram's eventual measure so the transcript does not jump when it resolves.
  loading: {
    position: "relative",
    marginBlock: "calc(var(--spacing) * 3.5)",
    display: "grid",
    height: "calc(var(--spacing) * 60)",
    minHeight: "calc(var(--spacing) * 25)",
    width: "100%",
    placeItems: "center",
    overflow: "hidden",
    borderRadius: radius.lg,
    borderWidth: "0.5px",
    borderStyle: "solid",
    borderColor: surface.fieldStrong,
    backgroundColor: surface.surface,
  },
  // The diagram is an SVG the renderer produces, so its own sizing is a DESCENDANT rule that
  // stays a utility — everything about the frame around it is here.
  stage: { overflowX: "auto", padding: space.s4, textAlign: "center", outline: "none" },
  pulse: {
    height: space.s8,
    width: space.s8,
    borderRadius: radius.card,
    backgroundColor: surface.surface3,
    animation: "var(--animate-pulse)",
  },
});

type MermaidRenderer = typeof import("beautiful-mermaid").renderMermaidSVG;
let rendererPromise: Promise<MermaidRenderer> | null = null;
function loadRenderer(): Promise<MermaidRenderer> {
  if (!rendererPromise) {
    rendererPromise = import("beautiful-mermaid").then((m) => m.renderMermaidSVG);
  }
  return rendererPromise;
}

interface Props {
  code: string;
}

type MermaidRenderResult = { status: "loading" | "error" | "rendered"; svg?: string };

interface SettledMermaidRender {
  code: string;
  tokenRevision: object;
  renderer: MermaidRenderer;
  result: MermaidRenderResult;
}

function readThemeColors(_tokenRevision: object) {
  const root = document.documentElement;
  const cs = getComputedStyle(root);
  const grab = (name: string, fallback: string) => cs.getPropertyValue(name).trim() || fallback;
  return {
    fg: grab("--color-text", "#e6e6e6"),
    muted: grab("--color-text-muted", "#9a9a9a"),
    line: grab("--color-text-faint", "#6f6f6f"),
    accent: grab("--color-accent", "#1ed760"),
    surface: grab("--color-surface-2", "#1f1f1f"),
    border: grab("--color-border", "#4d4d4d"),
  };
}

export function MermaidBlock({ code }: Props) {
  const t = useT();
  const fencedCode = useMemo(() => `\`\`\`mermaid\n${code}\n\`\`\``, [code]);
  const { copied, copy } = useCopyFeedback(fencedCode);
  const tokenRevision = useTokenRevision();
  const [debouncedCode] = useDebouncedValue(code, { wait: 300 });
  const isSettling = code !== debouncedCode;

  const [renderer, setRenderer] = useState<MermaidRenderer | null>(null);
  useEffect(() => {
    let alive = true;
    loadRenderer().then((fn) => {
      if (alive) setRenderer(() => fn);
    });
    return () => {
      alive = false;
    };
  }, []);

  const [settledRender, setSettledRender] = useState<SettledMermaidRender | null>(null);
  useEffect(() => {
    if (!renderer || isSettling) return;
    let cancelled = false;
    void Promise.resolve().then(() => {
      let result: MermaidRenderResult;
      try {
        const c = readThemeColors(tokenRevision);
        const start = performance.now();
        const svg = renderer(debouncedCode, {
          transparent: true,
          bg: c.surface,
          fg: c.fg,
          line: c.line,
          accent: c.accent,
          muted: c.muted,
          surface: c.surface,
          border: c.border,
        });
        measureMermaidRender(performance.now() - start);
        result = { status: "rendered", svg };
      } catch {
        result = { status: "error" };
      }
      if (!cancelled) setSettledRender({ code: debouncedCode, tokenRevision, renderer, result });
    });
    return () => {
      cancelled = true;
    };
  }, [debouncedCode, isSettling, tokenRevision, renderer]);
  const rendered =
    settledRender?.code === debouncedCode &&
    settledRender.tokenRevision === tokenRevision &&
    settledRender.renderer === renderer &&
    !isSettling
      ? settledRender.result
      : { status: "loading" as const };

  const [zoomed, setZoomed] = useState(false);

  if (rendered.status === "rendered") {
    const svg = rendered.svg!;
    return (
      <div
        className={cn(
          stylex.props(reveal.host).className,
          "relative isolate my-3.5 min-h-25 w-full rounded-lg border-[0.5px] border-field-strong bg-surface",
        )}
        data-markdown-copy="code-block"
        data-markdown-copy-text={fencedCode}
      >
        <div
          role="img"
          aria-label={t("markdown.diagram")}
          tabIndex={-1}
          dir="ltr"
          className={cn("[&_svg]:h-auto [&_svg]:max-w-full", stylex.props(mb.stage).className)}
          dangerouslySetInnerHTML={{ __html: svg }}
        />
        <div
          data-reveal="hover"
          className={cn(
            "absolute top-1 right-1 z-1 flex gap-1 transition-opacity",
            stylex.props(reveal.shown).className,
          )}
          data-markdown-copy="exclude"
        >
          <LightboxDialog
            open={zoomed}
            onOpenChange={setZoomed}
            title={t("markdown.diagram")}
            trigger={
              <IconButton
                icon="maximize"
                size="xs"
                quiet
                title={t("message.mermaid.enlarge")}
                aria-haspopup="dialog"
              />
            }
          >
            <div
              className="[&_svg]:mx-auto [&_svg]:block [&_svg]:max-w-none"
              dangerouslySetInnerHTML={{ __html: svg }}
            />
          </LightboxDialog>
          <IconButton
            icon={copied ? "check" : "copy"}
            size="xs"
            quiet
            onClick={() => void copy()}
            title={t(copied ? "message.mermaid.copied" : "message.mermaid.copy")}
            tone={copied ? "success" : undefined}
          />
        </div>
        <span className="sr-only">{t("message.mermaid.source")}</span>
        <pre className="sr-only whitespace-pre-wrap">{code}</pre>
      </div>
    );
  }

  if (rendered.status === "loading") {
    return (
      <div role="status" aria-label={t("message.mermaid.loading")} {...stylex.props(mb.loading)}>
        <span aria-hidden="true" {...stylex.props(mb.pulse)} />
      </div>
    );
  }

  return <ShikiCodeBlock lang="mermaid" code={code} />;
}
