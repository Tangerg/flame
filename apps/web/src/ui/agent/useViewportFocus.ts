import { type RefObject, useLayoutEffect } from "react";

export function useViewportFocus(viewportRef: RefObject<HTMLDivElement | null>, enabled: boolean) {
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
    const revealUnit = (target: Element, bounds: ReturnType<typeof frame>) => {
      const surface = target.closest('[data-slot="composer-root"]');
      if (!surface || !viewport.contains(surface)) return target;
      const height = surface.getBoundingClientRect().height;
      return height > 0 && height <= bounds.bottom - bounds.top + 0.5 ? surface : target;
    };
    const remember = () => {
      const active = document.activeElement;
      const bounds = frame();
      witnessState.value =
        active &&
        active !== viewport &&
        viewport.contains(active) &&
        visible(pressedTarget(active) ? active : revealUnit(active, bounds), bounds)
          ? active
          : null;
    };
    const retire = () => {
      witnessState.value = null;
    };
    const onWheel = (event: WheelEvent) => {
      const active = document.activeElement;
      // WebKit can consume an inner wheel before dispatch; actual outer scrolling retires in onScroll.
      if (
        active instanceof HTMLElement &&
        active !== viewport &&
        event.target instanceof Node &&
        active.contains(event.target) &&
        ["auto", "scroll"].includes(getComputedStyle(active).overflowY) &&
        active.scrollHeight > active.clientHeight &&
        event.deltaY !== 0 &&
        visible(active, frame())
      )
        return;
      retire();
    };
    const pressedTarget = (target: Element) =>
      pressedState.value &&
      (target.contains(pressedState.value) ||
        pressedState.value.closest("label, [data-focus-proxy]")?.contains(target));
    const reveal = () => {
      previousState.value = frame();
      // A larger frame can clamp native scroll before its delayed scroll event arrives.
      scrollTopState.value = viewport.scrollTop;
      if (!witnessState.value) return;
      if (
        witnessState.value !== document.activeElement ||
        !viewport.contains(witnessState.value) ||
        witnessState.value.getClientRects().length === 0
      ) {
        retire();
        return;
      }
      const target = revealUnit(witnessState.value, previousState.value);
      if (
        pressedTarget(witnessState.value) ||
        previousState.value.bottom <= previousState.value.top ||
        visible(target, previousState.value)
      )
        return;
      const bounds = target.getBoundingClientRect();
      const alignStart =
        bounds.top < previousState.value.top ||
        bounds.height > previousState.value.bottom - previousState.value.top;
      const delta = alignStart
        ? Math.floor(bounds.top - previousState.value.top)
        : Math.ceil(bounds.bottom - previousState.value.bottom);
      // Absolute offsets preserve WebKit's fractional page-zoom geometry.
      viewport.scrollTo({ top: viewport.scrollTop + delta, behavior: "instant" });
      scrollTopState.value = viewport.scrollTop;
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
      retire();
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
      const pressed = pressedState.value;
      pressedState.value = null;
      if (!pressed || !viewport.contains(pressed)) return;
      reveal();
      remember();
    };
    remember();
    const observer = new ResizeObserver(reveal);
    observer.observe(viewport);
    for (const child of viewport.children) observer.observe(child);
    viewport.addEventListener("scroll", onScroll, { passive: true });
    viewport.addEventListener("wheel", onWheel, { passive: true, capture: true });
    viewport.addEventListener("pointerdown", onPointerDown, true);
    document.addEventListener("focusin", onFocus);
    document.addEventListener("pointerup", clearPress, true);
    document.addEventListener("pointercancel", clearPress, true);
    return () => {
      observer.disconnect();
      viewport.removeEventListener("scroll", onScroll);
      viewport.removeEventListener("wheel", onWheel, true);
      viewport.removeEventListener("pointerdown", onPointerDown, true);
      document.removeEventListener("focusin", onFocus);
      document.removeEventListener("pointerup", clearPress, true);
      document.removeEventListener("pointercancel", clearPress, true);
    };
  }, [viewportRef, enabled]);
}
