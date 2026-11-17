import * as stylex from "@stylexjs/stylex";
import { useLayoutEffect, useRef } from "react";
import {
  TransformComponent,
  TransformWrapper,
  useControls,
  useTransformComponent,
  type ReactZoomPanPinchContentRef,
} from "react-zoom-pan-pinch";
import { ZoomIn, ZoomOut, Maximize } from "@flame/icons/react";
import { IconButton } from "./icon-button";
import { Button } from "./button";
import {
  color,
  radius,
  space,
  surface,
  type as typeStep,
} from "./tokens.stylex";
export interface ImageSource {
  readonly src: string;
  readonly alt: string;
  readonly title?: string;
  readonly width?: number;
  readonly height?: number;
}
export interface ImageViewportLabels {
  zoomIn: string;
  zoomOut: string;
  fit: string;
}
const styles = stylex.create({
  frame: {
    position: "relative",
    display: "flex",
    flexDirection: "column",
    flex: 1,
    minWidth: 0,
    minHeight: 0,
  },
  port: {
    minHeight: 0,
    flex: 1,
    overflow: "hidden",
    width: "100%",
    height: "100%",
  },
  image: {
    display: "block",
    maxWidth: "none",
    maxHeight: "none",
    borderRadius: radius.lg,
  },
  tray: {
    borderRadius: radius.floatingPanel,
    boxShadow: "var(--shadow-overlay)",
    position: "absolute",
    bottom: space.s3,
    left: "50%",
    zIndex: 1,
    display: "flex",
    translate: "-50% 0",
    alignItems: "center",
    gap: space.s1,
    backgroundColor: surface.mediaScrim,
    padding: space.s1,
    color: color.onMedia,
  },
});
function Controls({ labels }: { labels: ImageViewportLabels }) {
  const controls = useControls();
  const percentage = useTransformComponent(({ state }) =>
    Math.round(state.scale * 100),
  );
  return (
    <div {...stylex.props(styles.tray, typeStep.uiSm)}>
      <IconButton
        icon={ZoomOut}
        aria-label={labels.zoomOut}
        onClick={() => void controls.zoomOut()}
        variant="mediaTray"
        size="xl"
      />
      <Button onClick={() => void controls.centerView(1)}>{percentage}%</Button>
      <IconButton
        icon={ZoomIn}
        aria-label={labels.zoomIn}
        onClick={() => void controls.zoomIn()}
        variant="mediaTray"
        size="xl"
      />
      <IconButton
        icon={Maximize}
        aria-label={labels.fit}
        onClick={() => void controls.fitToView({ maxScale: 1 })}
        variant="mediaTray"
        size="xl"
      />
    </div>
  );
}
export function ImageViewport({
  image,
  label,
  labels,
}: {
  image: ImageSource;
  label: string;
  labels: ImageViewportLabels;
}) {
  const camera = useRef<ReactZoomPanPinchContentRef>(null);
  const imageElement = useRef<HTMLImageElement>(null);
  const viewportProps = {
    role: "region",
    "aria-label": label,
    "data-slot": "image-camera",
  } as const;
  useLayoutEffect(() => {
    if (imageElement.current?.complete)
      void camera.current?.fitToView({ maxScale: 1, animationTime: 0 });
  }, [image]);
  return (
    <div {...stylex.props(styles.frame)}>
      <TransformWrapper
        ref={camera}
        minScale={0.01}
        maxScale={16}
        doubleClick={{ disabled: true }}
        keyboard={{ disabled: false, animationTime: 0 }}
        onInit={(controls) => {
          void controls.fitToView({ maxScale: 1, animationTime: 0 });
        }}
      >
        <TransformComponent
          wrapperProps={viewportProps}
          wrapperStyle={{
            width: "100%",
            height: "100%",
            flex: 1,
            minHeight: 0,
          }}
          contentStyle={{ width: "max-content", height: "max-content" }}
        >
          <img
            key={image.src}
            ref={imageElement}
            src={image.src}
            alt={image.alt}
            title={image.title}
            width={image.width}
            height={image.height}
            draggable={false}
            onLoad={(event) => {
              if (event.currentTarget === imageElement.current)
                void camera.current?.fitToView({
                  maxScale: 1,
                  animationTime: 0,
                });
            }}
            {...stylex.props(styles.image)}
          />
        </TransformComponent>
        <Controls labels={labels} />
      </TransformWrapper>
    </div>
  );
}
