import * as stylex from "@stylexjs/stylex";
import { type PointerEvent, useRef, useState } from "react";
import { useT } from "@/lib/i18n";
import { Button, IconButton } from "@/ui";
import { space, type as typeStep } from "@/styles/tokens.stylex";

const ZOOM_STEPS = [0.5, 0.75, 1, 1.5, 2, 3] as const;

const styles = stylex.create({
  root: {
    display: "flex",
    height: "min(85vh, 900px)",
    width: "min(92vw, 1400px)",
    flexDirection: "column",
  },
  bar: {
    display: "flex",
    alignItems: "center",
    justifyContent: "flex-end",
    gap: space.s1,
    padding: space.s2,
  },
  percent: { minWidth: "4ch", textAlign: "center", fontVariantNumeric: "tabular-nums" },
  port: { minHeight: 0, flex: 1, overflow: "auto", cursor: "grab" },
  panning: { cursor: "grabbing", userSelect: "none" },
  fit: { display: "grid", height: "100%", placeItems: "center", padding: space.s4 },
  fitSvg: { maxHeight: "100%", maxWidth: "100%" },
  sized: { padding: space.s4 },
});

function naturalWidth(svg: string): number | null {
  const viewBox = /viewBox="[-\d.]+ [-\d.]+ ([\d.]+) [\d.]+"/.exec(svg);
  const width = viewBox?.[1] ?? /width="([\d.]+)"/.exec(svg)?.[1];
  return width ? Number(width) : null;
}

export function MermaidViewer({ svg }: { svg: string }) {
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
        <Button
          variant={scale === "fit" ? "soft" : "ghost"}
          size="sm"
          onClick={() => setScale("fit")}
        >
          {t("message.mermaid.fit")}
        </Button>
        <Button variant={scale === 1 ? "soft" : "ghost"} size="sm" onClick={() => setScale(1)}>
          100%
        </Button>
        <IconButton
          icon="zoom-out"
          size="sm"
          title={t("message.mermaid.zoomOut")}
          disabled={stepIndex === 0}
          focusableWhenDisabled
          onClick={() => zoom(-1)}
        />
        <span {...stylex.props(styles.percent, typeStep.uiSm)}>
          {scale === "fit" ? "—" : `${Math.round(scale * 100)}%`}
        </span>
        <IconButton
          icon="zoom-in"
          size="sm"
          title={t("message.mermaid.zoomIn")}
          disabled={stepIndex === ZOOM_STEPS.length - 1}
          focusableWhenDisabled
          onClick={() => zoom(1)}
        />
      </div>
      <div
        data-slot="mermaid-full"
        role="img"
        aria-label={t("markdown.diagram")}
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={endPan}
        onPointerCancel={endPan}
        {...stylex.props(styles.port, panning && styles.panning)}
      >
        {scale === "fit" ? (
          <div {...stylex.props(styles.fit)}>
            <div
              {...stylex.props(styles.fitSvg)}
              data-mermaid-fit=""
              dangerouslySetInnerHTML={{ __html: svg }}
            />
          </div>
        ) : (
          <div
            {...stylex.props(styles.sized)}
            style={width ? { width: width * scale } : undefined}
            data-mermaid-scale={scale}
            dangerouslySetInnerHTML={{ __html: svg }}
          />
        )}
      </div>
    </div>
  );
}
