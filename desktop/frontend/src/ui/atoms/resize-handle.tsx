import type { KeyboardEvent as ReactKeyboardEvent, PointerEvent as ReactPointerEvent } from "react";
import { useCallback, useEffect, useRef } from "react";
import type { SeparatorPrimitiveProps } from "@/ui/primitives";
import { SeparatorPrimitive } from "@/ui/primitives";

const STEP_PX = 8;
const COARSE_STEP_PX = 24;

const RESIZE_KEYS = ["ArrowLeft", "ArrowRight", "Home", "End"];

/**
 * Base UI has no split-pane primitive, so the whole gesture lives here: window-level
 * listeners (a drag must survive the cursor leaving a 10px target), release on
 * `pointercancel` as well as `pointerup`, keyboard stepping, and a live ARIA range. The
 * width goes straight onto the container as a custom property and the caller hears once, on
 * release — React state per pointer-move re-renders the pane at pointer frequency.
 */
export interface ResizeHandleProps extends Omit<
  SeparatorPrimitiveProps,
  "orientation" | "role" | "tabIndex" | "onPointerDown" | "onKeyDown" | "onKeyUp" | "onBlur"
> {
  edge: "start" | "end";
  /**
   * The PERSISTED width, not what could be read back from the container. A window resize
   * clamps the layout without touching the preference, so announcing the preference keeps
   * the ordering of two ResizeObserver callbacks from becoming observable.
   */
  value: number;
  container: (handle: HTMLElement) => HTMLElement | null;
  property: string;
  /** Live width a gesture starts from — a pane can render narrower than `property` asked. */
  read: (container: HTMLElement) => number;
  minWidth: number;
  maxWidth: (containerWidth: number) => number;
  onCommit: (width: number) => void;
  /** Set while resizing so a pane that animates its own width does not animate each step. */
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
  onCommit,
  resizingAttribute,
  ...props
}: ResizeHandleProps) {
  const handleRef = useRef<HTMLDivElement>(null);
  // Held so a gesture ending without a pointer event (unmount mid-drag, release while the
  // window was hidden) cannot leave `pointermove` attached to the window.
  const listenersRef = useRef<{ move: (event: PointerEvent) => void; up: () => void } | null>(null);
  // Remembered rather than looked up again: by the time an unmount clears the attribute
  // the handle is detached and can no longer find its own container.
  const markedRef = useRef<HTMLElement | null>(null);

  // Read through a ref: the pointer handlers are installed once per drag and must not
  // capture the render that started it.
  const paneRef = useRef({ container, property, read, minWidth, maxWidth, onCommit });
  useEffect(() => {
    paneRef.current = { container, property, read, minWidth, maxWidth, onCommit };
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
      const max = pane.maxWidth(element.clientWidth);
      handle.setAttribute("aria-valuemax", String(max));
      handle.setAttribute("aria-valuenow", String(clampWidth(value, pane.minWidth, max)));
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
        // Re-read the container each move: a window resized mid-drag moves the ceiling.
        width = clampWidth(startWidth + delta, pane.minWidth, pane.maxWidth(element.clientWidth));
        element.style.setProperty(pane.property, `${width}px`);
        handle.setAttribute("aria-valuenow", String(width));
      };
      const up = () => {
        detach();
        // A press that never moved is not a resize — committing one rewrote the
        // preference on every click of the handle.
        if (moved) paneRef.current.onCommit(width);
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

      const max = pane.maxWidth(element.clientWidth);
      const step = event.shiftKey ? COARSE_STEP_PX : STEP_PX;
      const grows = event.key === (edge === "end" ? "ArrowRight" : "ArrowLeft");
      const next =
        event.key === "Home"
          ? pane.minWidth
          : event.key === "End"
            ? max
            : clampWidth(pane.read(element) + (grows ? step : -step), pane.minWidth, max);

      // Arrow keys repeat while held, so the animation suppression has to last until
      // release rather than for one step.
      mark(element);
      element.style.setProperty(pane.property, `${next}px`);
      handle.setAttribute("aria-valuemax", String(max));
      handle.setAttribute("aria-valuenow", String(next));
      pane.onCommit(next);
    },
    [edge, mark],
  );

  // Blur as well as key-up: focus can leave the handle while a key is down, and the key-up
  // then lands elsewhere, leaving the pane permanently unable to animate.
  const releaseKeyboard = useCallback(() => {
    if (listenersRef.current) return;
    detach();
  }, [detach]);

  return (
    <SeparatorPrimitive
      {...props}
      ref={handleRef}
      orientation="vertical"
      tabIndex={0}
      aria-valuemin={minWidth}
      aria-valuenow={Math.round(value)}
      onPointerDown={onPointerDown}
      onKeyDown={onKeyDown}
      onKeyUp={releaseKeyboard}
      onBlur={releaseKeyboard}
    />
  );
}

function clampWidth(width: number, minWidth: number, maxWidth: number): number {
  return Math.round(Math.min(maxWidth, Math.max(minWidth, width)));
}
