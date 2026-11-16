import { Separator } from "@base-ui/react/separator";
import { type ComponentProps } from "react";
import {
  type KeyboardEvent as ReactKeyboardEvent,
  type PointerEvent as ReactPointerEvent,
} from "react";
import { useCallback, useLayoutEffect, useRef } from "react";
import { isImeKey, PointerSession, useContentVisible } from "@flame/ui";
const STEP_PX = 8;

const COARSE_STEP_PX = 24;

const RESIZE_KEYS = ["ArrowLeft", "ArrowRight", "Home", "End"];

export interface ResizeHandleProps extends Omit<
  ComponentProps<typeof Separator>,
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
  previewProperty: string;
  read: (container: HTMLElement) => number;
  minWidth: (containerWidth: number) => number;
  maxWidth: (containerWidth: number) => number;
  onCommit: (width: number, containerWidth: number) => void;
}

interface ResizeScope {
  readonly element: HTMLElement;
  readonly edge: "start" | "end";
  readonly previewProperty: string;
  readonly value: number;
  readonly width: number;
  readonly min: number;
  readonly max: number;
  readonly visible: boolean;
}

export function ResizeHandle({
  edge,
  value,
  container,
  previewProperty,
  read,
  minWidth,
  maxWidth,
  onCommit,
  ...props
}: ResizeHandleProps) {
  const visible = useContentVisible();
  const handleRef = useRef<HTMLDivElement>(null);
  const gesture = useRef<PointerSession | null>(null);
  const marked = useRef<{ element: HTMLElement; property: string } | null>(null);
  const observer = useRef<{ element: HTMLElement; observer: ResizeObserver } | null>(null);
  const scope = useRef<ResizeScope | null>(null);
  const pane = useRef({
    edge,
    value,
    container,
    previewProperty,
    read,
    minWidth,
    maxWidth,
    onCommit,
    visible,
  });
  const detach = useCallback(() => {
    if (marked.current) {
      marked.current.element.style.removeProperty(marked.current.property);
      marked.current.element.removeAttribute("data-resizing");
    }
    marked.current = null;
  }, []);
  const retire = useCallback(() => gesture.current?.retire(), []);
  const mark = useCallback(
    (element: HTMLElement) => {
      detach();
      element.setAttribute("data-resizing", "");
      marked.current = { element, property: pane.current.previewProperty };
    },
    [detach],
  );

  const reflect = useCallback(
    function reflectScope(): ResizeScope | null {
      const handle = handleRef.current;
      const current = pane.current;
      const element = handle ? current.container(handle) : null;
      if (!handle || !element) {
        retire();
        detach();
        observer.current?.observer.disconnect();
        observer.current = null;
        scope.current = null;
        return null;
      }
      const width = element.clientWidth;
      const min = current.minWidth(width);
      const max = current.maxWidth(width);
      const next: ResizeScope = {
        element,
        edge: current.edge,
        previewProperty: current.previewProperty,
        value: current.value,
        width,
        min,
        max,
        visible: current.visible,
      };
      const previous = scope.current;
      const changed =
        previous !== null &&
        (Object.keys(next) as (keyof ResizeScope)[]).some((key) => previous[key] !== next[key]);
      if (changed || !current.visible) retire();
      if (!previous || changed) scope.current = next;
      if (!current.visible) {
        detach();
        observer.current?.observer.disconnect();
        observer.current = null;
        return scope.current;
      }
      if (observer.current?.element !== element) {
        observer.current?.observer.disconnect();
        const nextObserver = new element.ownerDocument.defaultView!.ResizeObserver(() =>
          reflectScope(),
        );
        observer.current = { element, observer: nextObserver };
        nextObserver.observe(element);
      }
      if (!gesture.current?.active) {
        handle.setAttribute("aria-valuemin", String(min));
        handle.setAttribute("aria-valuemax", String(max));
        handle.setAttribute("aria-valuenow", String(clampWidth(current.value, min, max)));
      }
      return scope.current;
    },
    [retire, detach],
  );

  useLayoutEffect(() => () => retire(), [value, edge, previewProperty, visible, retire]);
  useLayoutEffect(() => {
    pane.current = {
      edge,
      value,
      container,
      previewProperty,
      read,
      minWidth,
      maxWidth,
      onCommit,
      visible,
    };
    reflect();
  });
  useLayoutEffect(
    () => () => {
      retire();
      observer.current?.observer.disconnect();
      observer.current = null;
      detach();
    },
    [retire, detach],
  );
  const bindHandle = useCallback(
    (node: HTMLDivElement | null) => {
      if (handleRef.current !== node) retire();
      handleRef.current = node;
    },
    [retire],
  );

  const onPointerDown = useCallback(
    (event: ReactPointerEvent<HTMLDivElement>) => {
      if (gesture.current?.active) return;
      const admitted = reflect();
      const handle = handleRef.current;
      if (!admitted?.visible || !handle) return;
      const current = pane.current;
      const { element } = admitted;
      const startWidth = current.read(element);
      let width = startWidth;
      let moved = false;
      let active: PointerSession | null = null;
      active = PointerSession.start(handle, event, {
        current: () =>
          scope.current === admitted &&
          pane.current.visible &&
          handleRef.current === handle &&
          handle.isConnected &&
          element.isConnected &&
          pane.current.container(handle) === element &&
          element.clientWidth === admitted.width,
        move(delta) {
          const change = admitted.edge === "end" ? delta.x : -delta.x;
          if (change === 0 && !moved) return;
          moved = true;
          width = clampWidth(startWidth + change, admitted.min, admitted.max);
          element.style.setProperty(admitted.previewProperty, `${width}px`);
          handle.setAttribute("aria-valuenow", String(width));
        },
        finish(outcome) {
          if (gesture.current === active) gesture.current = null;
          detach();
          handle.setAttribute(
            "aria-valuenow",
            String(clampWidth(pane.current.value, admitted.min, admitted.max)),
          );
          if (outcome === "complete" && moved && width !== startWidth)
            current.onCommit(width, admitted.width);
        },
      });
      if (!active?.active) return;
      gesture.current = active;
      event.preventDefault();
      mark(element);
    },
    [reflect, detach, mark],
  );

  const onKeyDown = useCallback(
    (event: ReactKeyboardEvent<HTMLDivElement>) => {
      if (
        event.defaultPrevented ||
        isImeKey(event.nativeEvent) ||
        event.altKey ||
        event.ctrlKey ||
        event.metaKey ||
        !RESIZE_KEYS.includes(event.key)
      )
        return;
      retire();
      const admitted = reflect();
      if (!admitted?.visible) return;
      const current = pane.current;
      event.preventDefault();
      const step = event.shiftKey ? COARSE_STEP_PX : STEP_PX;
      const grows = event.key === (current.edge === "end" ? "ArrowRight" : "ArrowLeft");
      const next =
        event.key === "Home"
          ? admitted.min
          : event.key === "End"
            ? admitted.max
            : clampWidth(current.value + (grows ? step : -step), admitted.min, admitted.max);
      mark(admitted.element);
      current.onCommit(next, admitted.width);
    },
    [retire, reflect, mark],
  );
  const releaseKeyboard = useCallback(() => {
    if (!gesture.current?.active) detach();
  }, [detach]);

  return (
    <Separator
      {...props}
      data-focus-inset=""
      ref={bindHandle}
      orientation="vertical"
      tabIndex={0}
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
