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
  const gestureRef = useRef<{ cancel(): void } | null>(null);
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

  const cancelGesture = useCallback(() => gestureRef.current?.cancel(), []);

  useEffect(
    () => () => {
      cancelGesture();
      detach();
    },
    [cancelGesture, detach, edge, property],
  );

  useEffect(() => {
    const handle = handleRef.current;
    if (!gestureRef.current || !handle) return;
    cancelGesture();
    const pane = paneRef.current;
    const element = pane.container(handle);
    if (!element) return;
    const containerWidth = element.clientWidth;
    const width = clampWidth(value, pane.minWidth(containerWidth), pane.maxWidth(containerWidth));
    element.style.setProperty(
      pane.property,
      pane.formatProperty(width, containerWidth),
      element.style.getPropertyPriority(pane.property),
    );
  }, [cancelGesture, value]);

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
      handle.setAttribute("aria-valuenow", String(clampWidth(value, min, max)));
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
      if (gestureRef.current) return;
      event.preventDefault();
      detach();

      const pane = paneRef.current;
      const pointerId = event.pointerId;
      const startX = event.clientX;
      const startWidth = pane.read(element);
      const containerWidth = element.clientWidth;
      const original = element.style.getPropertyValue(pane.property);
      const priority = element.style.getPropertyPriority(pane.property);
      let width = startWidth;
      let moved = false;

      const finish = (commit: boolean) => {
        if (!gestureRef.current) return;
        gestureRef.current = null;
        window.removeEventListener("pointermove", move);
        window.removeEventListener("pointerup", up);
        window.removeEventListener("pointercancel", canceled);
        window.removeEventListener("keydown", keydown, true);
        window.removeEventListener("blur", cancel);
        handle.removeEventListener("lostpointercapture", canceled);
        observer.disconnect();
        if (handle.hasPointerCapture(pointerId)) handle.releasePointerCapture(pointerId);
        detach();
        if (commit && moved && width !== startWidth) {
          pane.onCommit(width, containerWidth);
        } else {
          if (original) element.style.setProperty(pane.property, original, priority);
          else element.style.removeProperty(pane.property);
          handle.setAttribute("aria-valuenow", String(paneRef.current.read(element)));
        }
      };
      const cancel = () => finish(false);
      const move = (moveEvent: PointerEvent) => {
        if (moveEvent.pointerId !== pointerId) return;
        if (
          !element.isConnected ||
          element.clientWidth !== containerWidth ||
          paneRef.current.container(handle) !== element
        ) {
          cancel();
          return;
        }
        const delta = edge === "end" ? moveEvent.clientX - startX : startX - moveEvent.clientX;
        if (delta === 0 && !moved) return;
        moved = true;
        width = clampWidth(
          startWidth + delta,
          pane.minWidth(containerWidth),
          pane.maxWidth(containerWidth),
        );
        element.style.setProperty(pane.property, pane.formatProperty(width, containerWidth));
        handle.setAttribute("aria-valuenow", String(width));
      };
      const up = (upEvent: PointerEvent) => {
        if (upEvent.pointerId === pointerId) finish(element.clientWidth === containerWidth);
      };
      const canceled = (cancelEvent: PointerEvent) => {
        if (cancelEvent.pointerId === pointerId) cancel();
      };
      const keydown = (keyEvent: KeyboardEvent) => {
        if (keyEvent.key !== "Escape") return;
        keyEvent.preventDefault();
        keyEvent.stopPropagation();
        cancel();
      };
      const observer = new ResizeObserver(() => {
        if (element.clientWidth !== containerWidth) cancel();
      });
      observer.observe(element);
      gestureRef.current = { cancel };
      handle.setPointerCapture(pointerId);
      mark(element);
      window.addEventListener("pointermove", move);
      window.addEventListener("pointerup", up);
      window.addEventListener("pointercancel", canceled);
      window.addEventListener("keydown", keydown, true);
      window.addEventListener("blur", cancel);
      handle.addEventListener("lostpointercapture", canceled);
    },
    [detach, edge, mark],
  );

  const onKeyDown = useCallback(
    (event: ReactKeyboardEvent<HTMLDivElement>) => {
      if (!RESIZE_KEYS.includes(event.key)) return;
      cancelGesture();
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
    [cancelGesture, edge, mark],
  );

  const releaseKeyboard = useCallback(() => {
    if (gestureRef.current) return;
    detach();
  }, [detach]);

  return (
    <SeparatorPrimitive
      {...props}
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
