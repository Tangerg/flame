import { iconGlyph } from "@/ui/icons/index";
import * as stylex from "@stylexjs/stylex";
import {
  useCallback,
  useEffect,
  useEffectEvent,
  useId,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { useDebouncedValue } from "@tanstack/react-pacer";
import { LightboxDialog } from "@flame/ui";
import { CodeArtifact } from "@/ui/index";
import { reveal } from "@/ui/index";
import { IconButton } from "@flame/ui";
import { measureMermaidRender } from "@/lib/metrics";
import { useT } from "@/lib/i18n/index";
import { useContentVisible, useCopy } from "@flame/ui";
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

type MermaidRenderResult = { status: "error" } | { status: "rendered"; svg: string };

export function MermaidBlock({ code }: Props) {
  const t = useT();
  const visible = useContentVisible();
  const fencedCode = useMemo(() => `\`\`\`mermaid\n${code}\n\`\`\``, [code]);
  const copy = useCopy(fencedCode, t("message.code.copy"));
  const idPrefix = `mermaid-${useId().replace(/[^A-Za-z0-9_-]/g, "_")}`;
  const [debouncedCode] = useDebouncedValue(code, { wait: 300 });
  const isSettling = code !== debouncedCode;

  const [rendered, setRendered] = useState<MermaidRenderResult | null>(null);
  const [zoomed, setZoomed] = useState(false);
  const acceptRender = useEffectEvent((requested: string, result: MermaidRenderResult) => {
    if (requested !== code || requested !== debouncedCode) return;
    setRendered(result);
    if (result.status === "error") setZoomed(false);
  });
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
      if (!cancelled) acceptRender(debouncedCode, result);
    });
    return () => {
      cancelled = true;
    };
  }, [debouncedCode, isSettling, idPrefix]);
  const stage = useRef<HTMLDivElement>(null);
  const sourceBody = useRef<HTMLDivElement>(null);
  const portalFocus = useRef<Element>(null);
  const retiredRoot = useRef<HTMLDivElement>(null);
  const presentationRef = useCallback((root: HTMLDivElement | null) => {
    if (!root) return;
    if (retiredRoot.current === root) retiredRoot.current = null;
    return () => {
      const active = root.ownerDocument.activeElement;
      if (active && (root.contains(active) || active === portalFocus.current)) {
        retiredRoot.current = root;
      }
      portalFocus.current = null;
    };
  }, []);
  useLayoutEffect(() => {
    const root = retiredRoot.current;
    if (!root) return;
    retiredRoot.current = null;
    const target = rendered?.status === "rendered" ? stage.current : sourceBody.current;
    if (
      visible &&
      !root.isConnected &&
      target?.isConnected &&
      target.ownerDocument.activeElement === target.ownerDocument.body
    ) {
      target.focus({ preventScroll: true });
    }
  }, [rendered, visible]);

  if (rendered?.status === "rendered") {
    const svg = rendered.svg;
    return (
      <div
        ref={presentationRef}
        onFocusCapture={({ currentTarget, target }) => {
          // Portal events bubble through the diagram frame even though the dialog lives elsewhere.
          portalFocus.current = currentTarget.contains(target) ? null : target;
        }}
        {...stylex.props(reveal.host, artifactFrame.frame, mb.frame)}
        data-markdown-copy="code-block"
        data-markdown-copy-text={fencedCode}
        data-wide-markdown-block="true"
      >
        <div
          ref={stage}
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
            icon={iconGlyph("copy")}
            size="xs"
            quiet
            onClick={() => void copy()}
            aria-label={t("message.mermaid.copy")}
            tone={undefined}
          />
        </div>
        <span className="sr-only">{t("message.mermaid.source")}</span>
        <pre className={cn("sr-only", stylex.props(mb.sourceText).className)}>{code}</pre>
      </div>
    );
  }

  if (!rendered) {
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

  return (
    <CodeArtifact ref={presentationRef} sourceBodyRef={sourceBody} lang="mermaid" code={code} />
  );
}
