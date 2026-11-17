import * as stylex from "@stylexjs/stylex";
import { useState, type ReactNode } from "react";
import {
  TransformComponent,
  TransformWrapper,
  useTransformComponent,
  useControls,
} from "react-zoom-pan-pinch";
import { IconButton, Button } from "@flame/ui";
import { Minus, Plus, Maximize } from "@flame/icons/react";
import { previewStyles as styles } from "./previewStyles";
import { readCamera, saveCamera, type Size } from "./cameraPreferences";
import { messages } from "./messages";
import { localize, type LabLocale, type LabPanel } from "./story";
const excluded = ["lab-preview-island", "input", "textarea", "button", "a"];
function Controls({ locale }: { locale: LabLocale }) {
  const controls = useControls();
  const scale = useTransformComponent(({ state }) => state.scale);
  const t = (key: keyof typeof messages) => localize(messages[key], locale);
  return (
    <div {...stylex.props(styles.cameraToolbar)} role="toolbar" aria-label={t("canvas")}>
      <IconButton icon={Minus} aria-label={t("zoomOut")} onClick={() => void controls.zoomOut()} />
      <Button title={t("resetCamera")} onClick={() => void controls.centerView(1)}>
        {Math.round(scale * 100)}%
      </Button>
      <IconButton icon={Plus} aria-label={t("zoomIn")} onClick={() => void controls.zoomIn()} />
      <IconButton
        icon={Maximize}
        aria-label={t("fit")}
        onClick={() => void controls.fitToView({ maxScale: 1 })}
      />
    </div>
  );
}
export function PreviewCanvas({
  children,
  panel,
  locale,
  content,
}: {
  children: ReactNode;
  panel: LabPanel;
  locale: LabLocale;
  content: Size;
}) {
  const [saved] = useState(() => readCamera(panel));
  const t = (key: keyof typeof messages) => localize(messages[key], locale);
  return (
    <div {...stylex.props(styles.canvasShell)}>
      <TransformWrapper
        key={panel}
        minScale={0.01}
        maxScale={16}
        limitToBounds={false}
        initialPositionX={saved?.positionX}
        initialPositionY={saved?.positionY}
        initialScale={saved?.scale}
        panning={{ excluded, velocityDisabled: true }}
        wheel={{ excluded, activationKeys: ["Control", "Meta"] }}
        pinch={{ excluded }}
        trackPadPanning={{ excluded }}
        doubleClick={{ disabled: true }}
        keyboard={{ disabled: false, excluded }}
        onInit={(controls) => {
          if (!saved) void controls.fitToView({ maxScale: 1, animationTime: 0 });
        }}
        onTransform={(_controls, state) => saveCamera(panel, state)}
      >
        <div {...stylex.props(styles.canvas)} data-lab-pan-surface>
          <TransformComponent
            wrapperStyle={{ width: "100%", height: "100%" }}
            wrapperProps={{ role: "region", "aria-label": t("canvas") }}
            contentStyle={{ width: content.width, height: content.height }}
          >
            <div
              data-lab-canvas-content
              className="lab-preview-island"
              style={{ width: content.width, height: content.height }}
            >
              {children}
            </div>
          </TransformComponent>
        </div>
        <Controls locale={locale} />
      </TransformWrapper>
      <p id="lab-canvas-help" {...stylex.props(styles.canvasHelp)}>
        {t("canvasHelp")}
      </p>
    </div>
  );
}
