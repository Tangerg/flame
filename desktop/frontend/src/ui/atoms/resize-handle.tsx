import type { KeyboardEvent as ReactKeyboardEvent, PointerEvent as ReactPointerEvent } from "react";
import { useCallback, useEffect, useRef } from "react";
import type { SeparatorPrimitiveProps } from "@/ui/primitives";
import { SeparatorPrimitive } from "@/ui/primitives";

const STEP_PX = 8;
const COARSE_STEP_PX = 24;

const RESIZE_KEYS = ["ArrowLeft", "ArrowRight", "Home", "End"];

export interface ResizeHandleProps extends Omit<
  SeparatorPrimitiveProps,
  | "orientation"
  | "role"
  | "tabIndex"
  | "onPointerDown"
  | "onKeyDown"
  | "onKeyUp"
  | "onBlur"
  | "aria-valuenow"
> {
  edge: "start" | "end";
  value: number;
  container: (handle: HTMLElement) => HTMLElement | null;
  property: string;
  read: (container: HTMLElement) => number;
  minWidth: (containerWidth: number) => number;
  maxWidth: (containerWidth: number) => number;
  formatProperty?: (width: number, containerWidth: number) => string;
  onCommit: (width: number, containerWidth: number) => void;
  resizingAttribute?: string;
}

export function ResizeHandle({
  edge,
  value,
  container,
  property,
  read,
  minWidth,
  maxWidth,
  formatProperty = defaultFormatProperty,
  onCommit,
  resizingAttribute,
  ...props
}: ResizeHandleProps) {
  const handleRef = useRef<HTMLDivElement>(null);
  const listenersRef = useRef<{ move: (event: PointerEvent) => void; up: () => void } | null>(null);
  const markedRef = useRef<HTMLElement | null>(null);

  const paneRef = useRef({
    container,
    property,
    read,
    minWidth,
    maxWidth,
    formatProperty,
    onCommit,
  });
  useEffect(() => {
    paneRef.current = {
      container,
      property,
      read,
      minWidth,
      maxWidth,
      formatProperty,
      onCommit,
    };
  });

  const detach = useCallback(() => {
    const listeners = listenersRef.current;
    if (listeners) {
      window.removeEventListener("pointermove", listeners.move);
      window.removeEventListener("pointerup", listeners.up);
      window.removeEventListener("pointercancel", listeners.up);
      listenersRef.current = null;
    }
    const marked = markedRef.current;
    if (marked && resizingAttribute) marked.removeAttribute(resizingAttribute);
    markedRef.current = null;
  }, [resizingAttribute]);

  const mark = useCallback(
    (element: HTMLElement) => {
      if (!resizingAttribute) return;
      element.setAttribute(resizingAttribute, "");
      markedRef.current = element;
    },
    [resizingAttribute],
  );

  useEffect(() => detach, [detach]);

  useEffect(() => {
    const handle = handleRef.current;
    const element = handle ? paneRef.current.container(handle) : null;
    if (!handle || !element) return;
    const sync = () => {
      const pane = paneRef.current;
      const min = pane.minWidth(element.clientWidth);
      const max = pane.maxWidth(element.clientWidth);
      handle.setAttribute("aria-valuemin", String(min));
      handle.setAttribute("aria-valuemax", String(max));
      handle.setAttribute("aria-valuenow", String(clampWidth(pane.read(element), min, max)));
    };
    sync();
    const observer = new ResizeObserver(sync);
    observer.observe(element);
    return () => observer.disconnect();
  }, [value]);

  const onPointerDown = useCallback(
    (event: ReactPointerEvent<HTMLDivElement>) => {
      const handle = handleRef.current;
      const element = handle ? paneRef.current.container(handle) : null;
      if (!handle || !element || event.button !== 0) return;
      event.preventDefault();
      detach();

      const startX = event.clientX;
      const startWidth = paneRef.current.read(element);
      let width = startWidth;
      let moved = false;

      const move = (moveEvent: PointerEvent) => {
        const pane = paneRef.current;
        const delta = edge === "end" ? moveEvent.clientX - startX : startX - moveEvent.clientX;
        if (delta !== 0) moved = true;
        const containerWidth = element.clientWidth;
        width = clampWidth(
          startWidth + delta,
          pane.minWidth(containerWidth),
          pane.maxWidth(containerWidth),
        );
        element.style.setProperty(pane.property, pane.formatProperty(width, containerWidth));
        handle.setAttribute("aria-valuenow", String(width));
      };
      const up = () => {
        detach();
        if (moved) paneRef.current.onCommit(width, element.clientWidth);
      };

      mark(element);
      listenersRef.current = { move, up };
      window.addEventListener("pointermove", move);
      window.addEventListener("pointerup", up);
      window.addEventListener("pointercancel", up);
    },
    [detach, edge, mark],
  );

  const onKeyDown = useCallback(
    (event: ReactKeyboardEvent<HTMLDivElement>) => {
      if (!RESIZE_KEYS.includes(event.key)) return;
      const handle = handleRef.current;
      const pane = paneRef.current;
      const element = handle ? pane.container(handle) : null;
      if (!handle || !element) return;
      event.preventDefault();

      const containerWidth = element.clientWidth;
      const min = pane.minWidth(containerWidth);
      const max = pane.maxWidth(containerWidth);
      const step = event.shiftKey ? COARSE_STEP_PX : STEP_PX;
      const grows = event.key === (edge === "end" ? "ArrowRight" : "ArrowLeft");
      const next =
        event.key === "Home"
          ? min
          : event.key === "End"
            ? max
            : clampWidth(pane.read(element) + (grows ? step : -step), min, max);

      mark(element);
      element.style.setProperty(pane.property, pane.formatProperty(next, containerWidth));
      handle.setAttribute("aria-valuemin", String(min));
      handle.setAttribute("aria-valuemax", String(max));
      handle.setAttribute("aria-valuenow", String(next));
      pane.onCommit(next, containerWidth);
    },
    [edge, mark],
  );

  const releaseKeyboard = useCallback(() => {
    if (listenersRef.current) return;
    detach();
  }, [detach]);

  return (
    <SeparatorPrimitive
      {...props}
      // The handle sits ON the seam it drags, so an outward ring is always clipped by the pane
      // on one side of it. Arrow keys resize, so a keyboard user needs to see this one.
      data-focus-inset=""
      ref={handleRef}
      orientation="vertical"
      tabIndex={0}
      onPointerDown={onPointerDown}
      onKeyDown={onKeyDown}
      onKeyUp={releaseKeyboard}
      onBlur={releaseKeyboard}
    />
  );
}

function defaultFormatProperty(width: number): string {
  return `${width}px`;
}

function clampWidth(width: number, minWidth: number, maxWidth: number): number {
  return Math.round(Math.min(maxWidth, Math.max(minWidth, width)));
}
