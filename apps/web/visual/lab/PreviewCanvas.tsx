import * as stylex from "@stylexjs/stylex";
import { previewStyles as styles } from "./previewStyles";
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type ReactNode,
  type PointerEvent,
  type KeyboardEvent,
} from "react";
import { IconButton, Button } from "@flame/ui";
import { Minus, Plus, Maximize } from "@flame/icons/react";
import { Camera, CAMERA_LIMITS, readCamera, saveCamera, type Point, type Size } from "./camera";
import { messages } from "./messages";
import { localize, type LabLocale, type LabPanel } from "./story";

type Gesture = { camera: Camera; center: Point; distance: number };

function geometry(points: Map<number, Point>): { center: Point; distance: number } | null {
  const [first, second] = [...points.values()];
  if (!first) return null;
  if (!second) return { center: first, distance: 0 };
  return {
    center: { x: (first.x + second.x) / 2, y: (first.y + second.y) / 2 },
    distance: Math.hypot(first.x - second.x, first.y - second.y),
  };
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
  const viewport = useRef<HTMLDivElement>(null);
  const panSurface = useRef<HTMLButtonElement>(null);
  const [saved] = useState(() => readCamera(panel));
  const [camera, setCamera] = useState(() => saved ?? Camera.at());
  const current = useRef(camera);
  const frame = useRef<number | null>(null);
  const pointers = useRef(new Map<number, Point>());
  const gesture = useRef<Gesture | null>(null);
  const initialized = useRef(false);
  const previousContent = useRef(content);
  const { width: contentWidth, height: contentHeight } = content;
  const t = (key: keyof typeof messages) => localize(messages[key], locale);

  const move = useCallback((next: Camera) => {
    current.current = next;
    if (frame.current !== null) return;
    frame.current = requestAnimationFrame(() => {
      frame.current = null;
      setCamera(current.current);
    });
  }, []);

  const fit = useCallback(() => {
    const node = viewport.current;
    if (node)
      move(
        Camera.fit(
          { width: node.clientWidth, height: node.clientHeight },
          { width: contentWidth, height: contentHeight },
        ),
      );
  }, [contentWidth, contentHeight, move]);

  const center = () => {
    const node = viewport.current;
    return { x: (node?.clientWidth ?? 0) / 2, y: (node?.clientHeight ?? 0) / 2 };
  };

  const stop = useCallback(() => {
    const node = panSurface.current;
    for (const id of pointers.current.keys()) {
      if (node?.hasPointerCapture(id)) node.releasePointerCapture(id);
    }
    pointers.current.clear();
    gesture.current = null;
  }, []);

  useEffect(() => {
    const node = viewport.current;
    const pan = panSurface.current;
    if (!node || !pan) return;
    const resize = new ResizeObserver(() => {
      if (!initialized.current) {
        initialized.current = true;
        if (!saved) fit();
      }
    });
    resize.observe(node);
    const wheel = (event: WheelEvent) => {
      if (event.target !== pan || !(event.ctrlKey || event.metaKey)) return;
      event.preventDefault();
      const bounds = node.getBoundingClientRect();
      move(
        current.current.zoom(Math.exp(-event.deltaY * CAMERA_LIMITS.wheelSensitivity), {
          x: event.clientX - bounds.left,
          y: event.clientY - bounds.top,
        }),
      );
    };
    const hidden = () => {
      if (document.hidden) stop();
    };
    pan.addEventListener("wheel", wheel, { passive: false });
    window.addEventListener("blur", stop);
    document.addEventListener("visibilitychange", hidden);
    return () => {
      resize.disconnect();
      pan.removeEventListener("wheel", wheel);
      window.removeEventListener("blur", stop);
      document.removeEventListener("visibilitychange", hidden);
      stop();
      if (frame.current !== null) {
        cancelAnimationFrame(frame.current);
        frame.current = null;
      }
    };
  }, [fit, move, saved, stop]);

  useEffect(() => {
    const previous = previousContent.current;
    previousContent.current = { width: contentWidth, height: contentHeight };
    if (previous.width !== contentWidth || previous.height !== contentHeight) fit();
  }, [contentWidth, contentHeight, fit]);
  useEffect(() => {
    saveCamera(panel, camera);
  }, [panel, camera]);

  function rebase() {
    const next = geometry(pointers.current);
    gesture.current = next ? { ...next, camera: current.current } : null;
  }

  function down(event: PointerEvent<HTMLButtonElement>) {
    if (event.target !== event.currentTarget || event.button !== 0) return;
    event.currentTarget.focus({ preventScroll: true });
    const bounds = event.currentTarget.getBoundingClientRect();
    pointers.current.set(event.pointerId, {
      x: event.clientX - bounds.left,
      y: event.clientY - bounds.top,
    });
    event.currentTarget.setPointerCapture(event.pointerId);
    rebase();
  }

  function drag(event: PointerEvent<HTMLButtonElement>) {
    if (!pointers.current.has(event.pointerId) || !gesture.current) return;
    const bounds = event.currentTarget.getBoundingClientRect();
    pointers.current.set(event.pointerId, {
      x: event.clientX - bounds.left,
      y: event.clientY - bounds.top,
    });
    const next = geometry(pointers.current);
    if (!next) return;
    const start = gesture.current;
    const zoomed =
      start.distance > 0 && next.distance > 0
        ? start.camera.zoom(next.distance / start.distance, start.center)
        : start.camera;
    move(zoomed.pan({ x: next.center.x - start.center.x, y: next.center.y - start.center.y }));
  }

  function up(event: PointerEvent<HTMLButtonElement>) {
    if (!pointers.current.delete(event.pointerId)) return;
    if (event.currentTarget.hasPointerCapture(event.pointerId))
      event.currentTarget.releasePointerCapture(event.pointerId);
    rebase();
  }

  function key(event: KeyboardEvent<HTMLButtonElement>) {
    if (event.target !== event.currentTarget || event.altKey || event.ctrlKey || event.metaKey)
      return;
    const step = CAMERA_LIMITS.keyboardStep;
    let next: Camera;
    switch (event.key) {
      case "ArrowLeft":
        next = current.current.pan({ x: step, y: 0 });
        break;
      case "ArrowRight":
        next = current.current.pan({ x: -step, y: 0 });
        break;
      case "ArrowUp":
        next = current.current.pan({ x: 0, y: step });
        break;
      case "ArrowDown":
        next = current.current.pan({ x: 0, y: -step });
        break;
      case "+":
      case "=":
        next = current.current.zoom(CAMERA_LIMITS.zoomStep, center());
        break;
      case "-":
        next = current.current.zoom(1 / CAMERA_LIMITS.zoomStep, center());
        break;
      case "Home":
        event.preventDefault();
        fit();
        return;
      default:
        return;
    }
    event.preventDefault();
    move(next);
  }

  return (
    <div {...stylex.props(styles.canvasShell)}>
      <div {...stylex.props(styles.canvas)} ref={viewport} role="region" aria-label={t("canvas")}>
        <button
          type="button"
          data-lab-pan-surface
          {...stylex.props(styles.panSurface)}
          ref={panSurface}
          aria-label={t("canvas")}
          aria-describedby="lab-canvas-help"
          onPointerDown={down}
          onPointerMove={drag}
          onPointerUp={up}
          onPointerCancel={up}
          onLostPointerCapture={up}
          onKeyDown={key}
        />
        <div
          data-lab-canvas-content
          {...stylex.props(styles.canvasContent)}
          style={{
            width: content.width,
            height: content.height,
            transform: `translate(${camera.x}px, ${camera.y}px) scale(${camera.scale})`,
          }}
        >
          {children}
        </div>
      </div>
      <div {...stylex.props(styles.cameraToolbar)} aria-label={t("canvas")} role="toolbar">
        <IconButton
          icon={Minus}
          aria-label={t("zoomOut")}
          onClick={() => move(current.current.zoom(1 / CAMERA_LIMITS.zoomStep, center()))}
        />
        <Button
          size="sm"
          variant="ghost"
          title={t("resetCamera")}
          onClick={() =>
            move(
              Camera.at(
                (viewport.current!.clientWidth - content.width) / 2,
                (viewport.current!.clientHeight - content.height) / 2,
              ),
            )
          }
        >
          {Math.round(camera.scale * 100)}%
        </Button>
        <IconButton
          icon={Plus}
          aria-label={t("zoomIn")}
          onClick={() => move(current.current.zoom(CAMERA_LIMITS.zoomStep, center()))}
        />
        <IconButton icon={Maximize} aria-label={t("fit")} onClick={fit} />
      </div>
      <p id="lab-canvas-help" {...stylex.props(styles.canvasHelp)}>
        {t("canvasHelp")}
      </p>
    </div>
  );
}
