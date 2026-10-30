import * as stylex from "@stylexjs/stylex";
import { clsx as cn } from "clsx";
import {
  createElement,
  type ComponentPropsWithRef,
  type KeyboardEvent,
  type KeyboardEventHandler,
} from "react";
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
export function ScrollRegion({
  as: Tag = "div",
  label,
  children,
  className,
  styles: extra,
  onKeyDown,
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
  return createElement(
    Tag,
    {
      ...props,
      role: "region",
      "aria-label": label,
      tabIndex: props.tabIndex ?? 0,
      ...styled,
      className: cn(styled.className, className),
      onKeyDown: (event: KeyboardEvent<HTMLElement>) => {
        callerKeyDown?.(event);
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
      },
    },
    children,
  );
}
