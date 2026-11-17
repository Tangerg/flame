import { iconGlyph } from "@/ui/icons/index";
import * as stylex from "@stylexjs/stylex";
import { useId, useMemo, useState } from "react";
import { TransformWrapper, TransformComponent } from "react-zoom-pan-pinch";
import { useT } from "@/lib/i18n/index";
import { Button, IconButton } from "@flame/ui";
import { space, type as typeStep } from "@flame/ui/tokens.stylex";
import { copyMermaidSvg } from "./mermaid";
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
    alignItems: "center",
    flexShrink: 0,
    gap: space.s2,
    padding: space.s2,
  },
  port: { flex: 1, minHeight: 0, overflow: "hidden" },
});
export function MermaidViewer({ svg, onClose }: { svg: string; onClose(): void }) {
  const t = useT();
  const id = useId().replace(/[^A-Za-z0-9_-]/g, "_");
  const displayed = useMemo(() => copyMermaidSvg(svg, `viewer-${id}`), [svg, id]);
  const [hand, setHand] = useState(false);
  return (
    <div {...stylex.props(styles.root)}>
      <TransformWrapper
        key={svg}
        minScale={0.01}
        maxScale={16}
        panning={{ activationKeys: hand ? [] : [" "] }}
        doubleClick={{ disabled: true }}
        keyboard={{ disabled: false, excluded: ["text", "a", "button", "input", "textarea"] }}
        onInit={(controls) => {
          void controls.fitToView({ maxScale: 1, animationTime: 0 });
        }}
      >
        {(controls) => (
          <>
            <div {...stylex.props(styles.bar, typeStep.uiSm)}>
              <Button onClick={() => void controls.fitToView({ maxScale: 1 })}>
                {t("message.mermaid.fit")}
              </Button>
              <Button onClick={() => void controls.centerView(1)}>100%</Button>
              <IconButton
                icon={iconGlyph("zoom-out")}
                aria-label={t("message.mermaid.zoomOut")}
                onClick={() => void controls.zoomOut()}
              />
              <IconButton
                icon={iconGlyph("zoom-in")}
                aria-label={t("message.mermaid.zoomIn")}
                onClick={() => void controls.zoomIn()}
              />
              <Button aria-pressed={hand} onClick={() => setHand((value) => !value)}>
                {t("message.mermaid.pan")}
              </Button>
              <IconButton icon={iconGlyph("x")} aria-label={t("common.close")} onClick={onClose} />
            </div>
            <div {...stylex.props(styles.port)} data-slot="mermaid-full">
              <TransformComponent
                wrapperProps={{ role: "region", "aria-label": t("markdown.diagram") }}
                wrapperStyle={{ width: "100%", height: "100%" }}
                contentStyle={{
                  width: "max-content",
                  height: "max-content",
                  userSelect: hand ? "none" : "text",
                }}
              >
                <div
                  role="img"
                  aria-label={t("markdown.diagram")}
                  dangerouslySetInnerHTML={{ __html: displayed }}
                />
              </TransformComponent>
            </div>
          </>
        )}
      </TransformWrapper>
    </div>
  );
}
