import { type RefObject, useLayoutEffect } from "react";

export function useEmptyConversationFocus(
  viewportRef: RefObject<HTMLDivElement | null>,
  enabled: boolean,
) {
  useLayoutEffect(() => {
    const viewport = viewportRef.current;
    if (!enabled || !viewport) return;
    const document = viewport.ownerDocument;
    let witness: Element | null = null;
    let pressed: Element | null = null;
    let scrollTop = viewport.scrollTop;
    const frame = () => {
      const bounds = viewport.getBoundingClientRect();
      const style = getComputedStyle(viewport);
      return {
        top: bounds.top + (parseFloat(style.scrollPaddingTop) || 0),
        bottom: bounds.bottom - (parseFloat(style.scrollPaddingBottom) || 0),
        width: bounds.width,
        contentHeight: viewport.scrollHeight,
      };
    };
    let previous = frame();
    const visible = (target: Element, bounds: ReturnType<typeof frame>) => {
      const rect = target.getBoundingClientRect();
      return (
        rect.top >= bounds.top - 0.5 &&
        (rect.height > bounds.bottom - bounds.top
          ? rect.top < bounds.top + 1
          : rect.bottom <= bounds.bottom + 0.5)
      );
    };
    const remember = () => {
      const active = document.activeElement;
      witness =
        active && active !== viewport && viewport.contains(active) && visible(active, frame())
          ? active
          : null;
    };
    const retire = () => {
      witness = null;
    };
    const pressedTarget = (target: Element) =>
      pressed &&
      (target.contains(pressed) || pressed.closest("label, [data-focus-proxy]")?.contains(target));
    const reveal = () => {
      previous = frame();
      if (!witness) return;
      if (
        witness !== document.activeElement ||
        !viewport.contains(witness) ||
        witness.getClientRects().length === 0
      ) {
        retire();
        return;
      }
      if (pressedTarget(witness) || previous.bottom <= previous.top || visible(witness, previous))
        return;
      const bounds = witness.getBoundingClientRect();
      const alignStart =
        bounds.top < previous.top || bounds.height > previous.bottom - previous.top;
      const delta = alignStart
        ? Math.floor(bounds.top - previous.top)
        : Math.ceil(bounds.bottom - previous.bottom);
      // Absolute offsets preserve WebKit's fractional page-zoom geometry.
      viewport.scrollTo({ top: viewport.scrollTop + delta, behavior: "instant" });
    };
    const onScroll = (event: Event) => {
      if (event.target !== viewport) return;
      const moved = viewport.scrollTop !== scrollTop;
      scrollTop = viewport.scrollTop;
      const next = frame();
      // Native reflow can notify scrolling before ResizeObserver.
      if (
        !moved ||
        next.top !== previous.top ||
        next.bottom !== previous.bottom ||
        next.width !== previous.width ||
        next.contentHeight !== previous.contentHeight
      )
        return;
      remember();
    };
    const onFocus = () => {
      const active = document.activeElement;
      if (!active || active === viewport || !viewport.contains(active)) {
        retire();
        return;
      }
      if (!pressedTarget(active)) {
        witness = active;
        reveal();
      }
      remember();
    };
    const onPointerDown = (event: PointerEvent) => {
      pressed = event.target instanceof Element ? event.target : null;
    };
    const clearPress = () => {
      pressed = null;
    };
    remember();
    const observer = new ResizeObserver(reveal);
    observer.observe(viewport);
    for (const child of viewport.children) observer.observe(child);
    viewport.addEventListener("scroll", onScroll, { passive: true });
    viewport.addEventListener("wheel", retire, { passive: true, capture: true });
    viewport.addEventListener("pointerdown", onPointerDown, true);
    document.addEventListener("focusin", onFocus);
    document.addEventListener("pointerup", clearPress, true);
    document.addEventListener("pointercancel", clearPress, true);
    return () => {
      observer.disconnect();
      viewport.removeEventListener("scroll", onScroll);
      viewport.removeEventListener("wheel", retire, true);
      viewport.removeEventListener("pointerdown", onPointerDown, true);
      document.removeEventListener("focusin", onFocus);
      document.removeEventListener("pointerup", clearPress, true);
      document.removeEventListener("pointercancel", clearPress, true);
    };
  }, [viewportRef, enabled]);
}
