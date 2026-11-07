import * as stylex from "@stylexjs/stylex";
import { clsx as cn } from "clsx";
import {
  createElement,
  useImperativeHandle,
  useLayoutEffect,
  useRef,
  type ComponentPropsWithRef,
  type KeyboardEvent,
  type KeyboardEventHandler,
  type Ref,
} from "react";
import { useContentVisible } from "./content-visibility";
import { space } from "./tokens.stylex";

const styles = stylex.create({
  region: {
    minWidth: 0,
    maxWidth: "100%",
    overflow: "auto",
    overscrollBehavior: "contain auto",
    paddingBlock: space.s1,
  },
});

// Native page zoom reports fractional CSS pixels at a rounded scroll offset.
const SUBPIXEL = 0.5;

interface HorizontalFrame {
  clientWidth: number;
  scrollWidth: number;
  targetWidth: number;
}

function horizontalPlacement(region: HTMLElement, target: Element) {
  const left = region.getBoundingClientRect().left + region.clientLeft;
  const right = left + region.clientWidth;
  const bounds = target.getBoundingClientRect();
  const frame: HorizontalFrame = {
    clientWidth: region.clientWidth,
    scrollWidth: region.scrollWidth,
    targetWidth: bounds.width,
  };
  if (
    region.clientWidth <= 0 ||
    bounds.width <= 0 ||
    bounds.width > region.clientWidth + SUBPIXEL
  )
    return { frame, displacement: null };
  // Whole pixels keep a truncated scroll offset from re-clipping a fractional edge.
  const displacement =
    bounds.left < left - SUBPIXEL
      ? Math.floor(bounds.left - left)
      : bounds.right > right + SUBPIXEL
        ? Math.ceil(bounds.right - right)
        : 0;
  return { frame, displacement };
}

// Some native hosts do not pan a focused scroller with arrows, so the reader owns the transition.
export function scrollWithHorizontalArrow(event: KeyboardEvent<HTMLElement>) {
  if (
    event.defaultPrevented ||
    event.target !== event.currentTarget ||
    event.altKey ||
    event.ctrlKey ||
    event.metaKey ||
    event.shiftKey ||
    event.nativeEvent.isComposing ||
    (event.key !== "ArrowLeft" && event.key !== "ArrowRight")
  )
    return;
  const region = event.currentTarget;
  if (region.clientWidth <= 0 || region.scrollWidth <= region.clientWidth)
    return;
  event.preventDefault();
  event.stopPropagation();
  region.scrollLeft +=
    region.clientWidth * 0.1 * (event.key === "ArrowLeft" ? -1 : 1);
}

export function ScrollRegion({
  as: Tag = "div",
  label,
  children,
  className,
  styles: extra,
  onKeyDown,
  ref,
  ...props
}: (
  | (ComponentPropsWithRef<"div"> & { as?: "div" })
  | (ComponentPropsWithRef<"span"> & { as: "span" })
) & {
  label: string;
  styles?: stylex.StyleXArray<stylex.StyleXStyles | null | false>;
}) {
  const styled = stylex.props(styles.region, extra);
  const callerKeyDown: KeyboardEventHandler<HTMLElement> | undefined =
    onKeyDown;
  const regionRef = useRef<HTMLElement>(null);
  useImperativeHandle(
    ref as Ref<HTMLElement> | undefined,
    () => regionRef.current!,
    [Tag],
  );
  const visible = useContentVisible();
  useLayoutEffect(() => {
    const region = regionRef.current;
    if (!visible || !region) return;
    const document = region.ownerDocument;
    // The focused descendant stays revealed until a deliberate scroll leaves it behind.
    let focused: Element | null = null;
    let following = false;
    let frame: HorizontalFrame | null = null;
    let position = region.scrollLeft;
    let pressed: Element | null = null;
    const follow = () => {
      if (!focused || !following) return;
      if (focused !== document.activeElement || !region.contains(focused)) {
        track(null);
        return;
      }
      const placement = horizontalPlacement(region, focused);
      const pointerFocus =
        pressed &&
        (focused.contains(pressed) ||
          pressed.closest("label, [data-focus-proxy]")?.contains(focused));
      // Moving a pressed control before pointerup can swallow its activation.
      if (placement.displacement && !pointerFocus)
        region.scrollTo({
          left: region.scrollLeft + placement.displacement,
          behavior: "instant",
        });
      frame = placement.frame;
      position = region.scrollLeft;
    };
    const observer = new ResizeObserver(follow);
    const track = (target: Element | null) => {
      if (focused) observer.unobserve(focused);
      focused = target;
      following = target !== null;
      frame = null;
      // The target's initial observation reveals it after native and Panel focus scrolling.
      if (target) observer.observe(target, { box: "border-box" });
    };
    observer.observe(region, { box: "border-box" });
    const initial = document.activeElement;
    if (initial && initial !== region && region.contains(initial)) {
      track(initial);
      // Revealed content keeps its reading position; only visible focus resumes following.
      following = horizontalPlacement(region, initial).displacement === 0;
    }
    const onFocusIn = (event: FocusEvent) => {
      // Window reactivation refocuses the same control without a new entry.
      if (event.target === focused) return;
      track(
        event.target instanceof Element && event.target !== region
          ? event.target
          : null,
      );
    };
    const onFocusOut = (event: FocusEvent) => {
      if (event.target === document.activeElement) return;
      const next = event.relatedTarget;
      if (!(next instanceof Node && region.contains(next))) pressed = null;
      track(null);
    };
    const onPointerDown = (event: PointerEvent) => {
      pressed = event.target instanceof Element ? event.target : null;
    };
    const release = () => {
      pressed = null;
    };
    const onWheel = (event: WheelEvent) => {
      if (event.deltaX !== 0 || (event.shiftKey && event.deltaY !== 0))
        following = false;
    };
    const onScroll = () => {
      const moved = region.scrollLeft !== position;
      position = region.scrollLeft;
      if (!moved || !focused || !following || !frame) return;
      const placement = horizontalPlacement(region, focused);
      // Reflow can notify before ResizeObserver delivery.
      if (
        placement.frame.clientWidth !== frame.clientWidth ||
        placement.frame.scrollWidth !== frame.scrollWidth ||
        placement.frame.targetWidth !== frame.targetWidth
      )
        follow();
      else if (placement.displacement !== 0) following = false;
    };
    region.addEventListener("focusin", onFocusIn);
    region.addEventListener("focusout", onFocusOut);
    region.addEventListener("pointerdown", onPointerDown, true);
    region.addEventListener("wheel", onWheel, { passive: true });
    region.addEventListener("scroll", onScroll, { passive: true });
    document.addEventListener("pointerup", release, true);
    document.addEventListener("pointercancel", release, true);
    return () => {
      observer.disconnect();
      region.removeEventListener("focusin", onFocusIn);
      region.removeEventListener("focusout", onFocusOut);
      region.removeEventListener("pointerdown", onPointerDown, true);
      region.removeEventListener("wheel", onWheel);
      region.removeEventListener("scroll", onScroll);
      document.removeEventListener("pointerup", release, true);
      document.removeEventListener("pointercancel", release, true);
    };
  }, [visible, Tag]);
  return createElement(
    Tag,
    {
      ...props,
      ref: regionRef,
      role: "region",
      "aria-label": label,
      tabIndex: props.tabIndex ?? 0,
      ...styled,
      className: cn(styled.className, className),
      onKeyDown: (event: KeyboardEvent<HTMLElement>) => {
        callerKeyDown?.(event);
        scrollWithHorizontalArrow(event);
      },
    },
    children,
  );
}
