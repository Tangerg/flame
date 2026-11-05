import { type RefObject, useLayoutEffect } from "react";

export function useEmptyConversationFocus(
  viewportRef: RefObject<HTMLDivElement | null>,
  enabled: boolean,
) {
  useLayoutEffect(() => {
    const viewport = viewportRef.current;
    if (!enabled || !viewport) return;
    const document = viewport.ownerDocument;
    const witnessState: { value: Element | null } = { value: null };
    const pressedState: { value: Element | null } = { value: null };
    const scrollTopState = { value: viewport.scrollTop };
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
    const previousState = { value: frame() };
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
      witnessState.value =
        active && active !== viewport && viewport.contains(active) && visible(active, frame())
          ? active
          : null;
    };
    const retire = () => {
      witnessState.value = null;
    };
    const pressedTarget = (target: Element) =>
      pressedState.value &&
      (target.contains(pressedState.value) ||
        pressedState.value.closest("label, [data-focus-proxy]")?.contains(target));
    const reveal = () => {
      previousState.value = frame();
      if (!witnessState.value) return;
      if (
        witnessState.value !== document.activeElement ||
        !viewport.contains(witnessState.value) ||
        witnessState.value.getClientRects().length === 0
      ) {
        retire();
        return;
      }
      if (
        pressedTarget(witnessState.value) ||
        previousState.value.bottom <= previousState.value.top ||
        visible(witnessState.value, previousState.value)
      )
        return;
      const bounds = witnessState.value.getBoundingClientRect();
      const alignStart =
        bounds.top < previousState.value.top ||
        bounds.height > previousState.value.bottom - previousState.value.top;
      const delta = alignStart
        ? Math.floor(bounds.top - previousState.value.top)
        : Math.ceil(bounds.bottom - previousState.value.bottom);
      // Absolute offsets preserve WebKit's fractional page-zoom geometry.
      viewport.scrollTo({ top: viewport.scrollTop + delta, behavior: "instant" });
    };
    const onScroll = (event: Event) => {
      if (event.target !== viewport) return;
      const moved = viewport.scrollTop !== scrollTopState.value;
      scrollTopState.value = viewport.scrollTop;
      const next = frame();
      // Native reflow can notify scrolling before ResizeObserver.
      if (
        !moved ||
        next.top !== previousState.value.top ||
        next.bottom !== previousState.value.bottom ||
        next.width !== previousState.value.width ||
        next.contentHeight !== previousState.value.contentHeight
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
        witnessState.value = active;
        reveal();
      }
      remember();
    };
    const onPointerDown = (event: PointerEvent) => {
      pressedState.value = event.target instanceof Element ? event.target : null;
    };
    const clearPress = () => {
      pressedState.value = null;
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
