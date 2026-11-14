import * as stylex from "@stylexjs/stylex";
import { useCallback, useLayoutEffect, useRef, type PointerEvent } from "react";
import { useContentVisible } from "./content-visibility";
import { PointerSession } from "./pointer-session";

export const scrollPan = stylex.create({
  viewport: {
    touchAction: "none",
    cursor: {
      default: null,
      ":is([data-pannable])": "grab",
      ":is([data-panning])": "grabbing",
    },
    userSelect: { default: null, ":is([data-panning])": "none" },
  },
});

export function useScrollPan(source: unknown, enabled: boolean) {
  const visible = useContentVisible();
  const viewport = useRef<HTMLElement | null>(null);
  const session = useRef<PointerSession | null>(null);
  const retire = useCallback(() => session.current?.retire(), []);
  const bind = useCallback(
    (node: HTMLElement | null) => {
      if (viewport.current === node) return;
      retire();
      viewport.current = node;
    },
    [retire],
  );
  useLayoutEffect(() => () => retire(), [source, enabled, visible, retire]);

  const onPointerDown = useCallback(
    (event: PointerEvent<HTMLElement>) => {
      const node = event.currentTarget;
      if (
        !enabled ||
        !visible ||
        viewport.current !== node ||
        session.current?.active
      )
        return;
      const left = node.scrollLeft;
      const top = node.scrollTop;
      const width = node.clientWidth;
      const height = node.clientHeight;
      let observer: ResizeObserver | undefined;
      let active: PointerSession | null = null;
      active = PointerSession.start(node, event, {
        current: () =>
          node.isConnected &&
          viewport.current === node &&
          node.clientWidth === width &&
          node.clientHeight === height &&
          !node.closest("[hidden], [inert]"),
        move(delta) {
          node.scrollLeft = left - delta.x;
          node.scrollTop = top - delta.y;
        },
        finish(outcome) {
          if (session.current === active) session.current = null;
          observer?.disconnect();
          node.removeAttribute("data-panning");
          if (outcome === "cancel") {
            node.scrollLeft = left;
            node.scrollTop = top;
          }
        },
      });
      if (!active?.active) return;
      session.current = active;
      observer = new node.ownerDocument.defaultView!.ResizeObserver(() => {
        if (node.clientWidth !== width || node.clientHeight !== height)
          active?.retire();
      });
      observer.observe(node);
      event.preventDefault();
      node.setAttribute("data-panning", "");
      node.focus({ preventScroll: true });
    },
    [enabled, visible],
  );
  return { ref: bind, onPointerDown, retire };
}
