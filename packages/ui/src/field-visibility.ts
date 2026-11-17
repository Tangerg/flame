import { useLayoutEffect, type RefObject } from "react";
import { useContentVisible } from "./content-visibility";

// A form viewport owns only its own offset. Nested readers retain their own scrolling.
export function useFieldVisibility(viewportRef: RefObject<HTMLElement | null>) {
  const visible = useContentVisible();
  useLayoutEffect(() => {
    const viewport = viewportRef.current;
    if (!visible || !viewport) return;
    let field: HTMLElement | null = null;
    let following = false;
    let pressed = false;
    let frame: number | undefined;
    const ownedField = (target: EventTarget | null) => {
      if (
        !(target instanceof HTMLElement) ||
        !target.matches(
          "input, textarea, button, [role=combobox], [role=checkbox], [role=radio], [role=switch]",
        )
      )
        return null;
      for (
        let parent = target.parentElement;
        parent !== viewport;
        parent = parent?.parentElement ?? null
      ) {
        if (!parent) return null;
        if (
          /auto|scroll/.test(getComputedStyle(parent).overflowY) &&
          parent.scrollHeight > parent.clientHeight
        )
          return null;
      }
      return target;
    };
    const reveal = () => {
      if (
        !following ||
        pressed ||
        !field ||
        field !== viewport.ownerDocument.activeElement
      )
        return;
      const box = viewport.getBoundingClientRect();
      const padding = getComputedStyle(viewport);
      const top =
        box.top +
        viewport.clientTop +
        (parseFloat(padding.scrollPaddingTop) || 0);
      const bottom =
        Math.min(
          box.bottom - (parseFloat(padding.borderBottomWidth) || 0),
          box.top + viewport.clientTop + viewport.clientHeight,
        ) - (parseFloat(padding.scrollPaddingBottom) || 0);
      const bounds = field.getBoundingClientRect();
      // Oversized fields keep their native caret scrolling instead of aligning their box.
      if (bounds.height > bottom - top) return;
      const delta =
        bounds.top < top
          ? Math.floor(bounds.top - top)
          : bounds.bottom > bottom
            ? Math.ceil(bounds.bottom - bottom)
            : 0;
      if (delta) viewport.scrollBy({ top: delta, behavior: "instant" });
    };
    const observer = new ResizeObserver(reveal);
    const observe = () => {
      observer.disconnect();
      observer.observe(viewport);
      for (const child of viewport.children) observer.observe(child);
    };
    observe();
    const children = new MutationObserver(observe);
    children.observe(viewport, { childList: true });
    const focus = (event: FocusEvent) => {
      field = ownedField(event.target);
      following = field !== null;
      reveal();
    };
    const wheel = (event: WheelEvent) => {
      if (event.deltaY) following = false;
    };
    const pointer = (event: PointerEvent) => {
      pressed = true;
      if (event.target === viewport) following = false;
    };
    const release = () => {
      pressed = false;
      if (frame !== undefined) cancelAnimationFrame(frame);
      frame = requestAnimationFrame(reveal);
    };
    const key = (event: KeyboardEvent) => {
      if (
        event.target === viewport &&
        [
          "PageUp",
          "PageDown",
          "Home",
          "End",
          "ArrowUp",
          "ArrowDown",
          " ",
        ].includes(event.key)
      )
        following = false;
    };
    field = ownedField(viewport.ownerDocument.activeElement);
    following = field !== null;
    viewport.addEventListener("focusin", focus);
    viewport.addEventListener("wheel", wheel, { passive: true });
    viewport.addEventListener("pointerdown", pointer, true);
    viewport.addEventListener("keydown", key);
    viewport.ownerDocument.addEventListener("pointerup", release, true);
    viewport.ownerDocument.addEventListener("pointercancel", release, true);
    return () => {
      observer.disconnect();
      children.disconnect();
      if (frame !== undefined) cancelAnimationFrame(frame);
      viewport.removeEventListener("focusin", focus);
      viewport.removeEventListener("wheel", wheel);
      viewport.removeEventListener("pointerdown", pointer, true);
      viewport.removeEventListener("keydown", key);
      viewport.ownerDocument.removeEventListener("pointerup", release, true);
      viewport.ownerDocument.removeEventListener(
        "pointercancel",
        release,
        true,
      );
    };
  }, [visible, viewportRef]);
}
