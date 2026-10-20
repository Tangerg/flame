import { iconGlyph } from "@/ui/icons/index";
import * as stylex from "@stylexjs/stylex";
import { type PointerEvent, useRef, useState } from "react";
import { useT } from "@/lib/i18n/index";
import { Button, IconButton, ScrollRegion } from "@flame/ui";
import { space, type as typeStep, weight } from "@flame/ui/tokens.stylex";
const ZOOM_STEPS = [0.5, 0.75, 1, 1.5, 2, 3] as const;

const styles = stylex.create({
  root: {
    display: "flex",
    minHeight: 0,
    minWidth: 0,
    height: "100%",
    width: "100%",
    flexDirection: "column",
  },
  bar: {
    display: "flex",
    flexWrap: "wrap",
    flexShrink: 0,
    alignItems: "center",
    gap: space.s2,
    paddingInline: space.s3,
    paddingBlock: space.s2,
  },
  title: { flex: "1 0 auto", fontWeight: weight.medium },
  actions: {
    display: "flex",
    flexWrap: "wrap",
    alignItems: "center",
    gap: space.s2,
    maxWidth: "100%",
    marginInlineStart: "auto",
  },
  zoom: { display: "flex", alignItems: "center", gap: space.s1 },
  percent: { minWidth: "4ch", textAlign: "center", fontVariantNumeric: "tabular-nums" },
  port: { minHeight: 0, flex: 1, padding: 0, overscrollBehavior: "contain" },
  pannable: { cursor: "grab" },
  panning: { cursor: "grabbing", userSelect: "none" },
  fit: {
    display: "flex",
    height: "100%",
    alignItems: "center",
    justifyContent: "center",
    padding: space.s4,
  },
  sized: { boxSizing: "content-box", padding: space.s4 },
});

function naturalWidth(svg: string): number | null {
  const viewBox = /viewBox="[-\d.]+ [-\d.]+ ([\d.]+) [\d.]+"/.exec(svg);
  const width = viewBox?.[1] ?? /width="([\d.]+)"/.exec(svg)?.[1];
  return width ? Number(width) : null;
}

export function MermaidViewer({ svg, onClose }: { svg: string; onClose: () => void }) {
  const t = useT();
  const [scale, setScale] = useState<number | "fit">("fit");
  const [panning, setPanning] = useState(false);
  const drag = useRef<{ x: number; y: number; left: number; top: number } | null>(null);
  const width = naturalWidth(svg);
  const stepIndex = scale === "fit" ? -1 : ZOOM_STEPS.indexOf(scale as (typeof ZOOM_STEPS)[number]);

  const zoom = (direction: 1 | -1) => {
    const from = stepIndex === -1 ? ZOOM_STEPS.indexOf(1) : stepIndex;
    const next = Math.min(ZOOM_STEPS.length - 1, Math.max(0, from + direction));
    setScale(ZOOM_STEPS[next]!);
  };

  const onPointerDown = (event: PointerEvent<HTMLDivElement>) => {
    if (scale === "fit") return;
    const port = event.currentTarget;
    drag.current = {
      x: event.clientX,
      y: event.clientY,
      left: port.scrollLeft,
      top: port.scrollTop,
    };
    port.setPointerCapture(event.pointerId);
    setPanning(true);
  };
  const onPointerMove = (event: PointerEvent<HTMLDivElement>) => {
    const start = drag.current;
    if (!start) return;
    event.currentTarget.scrollLeft = start.left - (event.clientX - start.x);
    event.currentTarget.scrollTop = start.top - (event.clientY - start.y);
  };
  const endPan = () => {
    drag.current = null;
    setPanning(false);
  };

  return (
    <div {...stylex.props(styles.root)}>
      <div {...stylex.props(styles.bar)}>
        <span {...stylex.props(styles.title, typeStep.uiMd)}>{t("markdown.diagram")}</span>
        <div {...stylex.props(styles.actions)}>
          <Button
            variant={scale === "fit" ? "soft" : "ghost"}
            size="sm"
            onClick={() => setScale("fit")}
          >
            {t("message.mermaid.fit")}
          </Button>
          <div {...stylex.props(styles.zoom)}>
            <IconButton
              icon={iconGlyph("zoom-out")}
              size="sm"
              aria-label={t("message.mermaid.zoomOut")}
              disabled={stepIndex === 0}
              focusableWhenDisabled
              onClick={() => zoom(-1)}
            />
            <Button
              variant={scale === 1 ? "soft" : "ghost"}
              size="sm"
              title={t("message.mermaid.actualSize")}
              onClick={() => setScale(1)}
              className={stylex.props(styles.percent).className}
            >
              {`${Math.round((scale === "fit" ? 1 : scale) * 100)}%`}
            </Button>
            <IconButton
              icon={iconGlyph("zoom-in")}
              size="sm"
              aria-label={t("message.mermaid.zoomIn")}
              disabled={stepIndex === ZOOM_STEPS.length - 1}
              focusableWhenDisabled
              onClick={() => zoom(1)}
            />
          </div>
          <IconButton
            icon={iconGlyph("x")}
            size="sm"
            aria-label={t("common.close")}
            onClick={onClose}
          />
        </div>
      </div>
      <ScrollRegion
        data-slot="mermaid-full"
        data-focus-inset=""
        label={t("markdown.diagram")}
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={endPan}
        onPointerCancel={endPan}
        styles={[styles.port, scale !== "fit" && styles.pannable, panning && styles.panning]}
      >
        {scale === "fit" ? (
          <div
            role="img"
            aria-label={t("markdown.diagram")}
            {...stylex.props(styles.fit)}
            data-mermaid-fit=""
            dangerouslySetInnerHTML={{ __html: svg }}
          />
        ) : (
          <div
            role="img"
            aria-label={t("markdown.diagram")}
            {...stylex.props(styles.sized)}
            style={width ? { width: width * scale } : undefined}
            data-mermaid-scale={scale}
            dangerouslySetInnerHTML={{ __html: svg }}
          />
        )}
      </ScrollRegion>
    </div>
  );
}
