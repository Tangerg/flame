import * as stylex from "@stylexjs/stylex";
import { clsx as cn } from "clsx";
import { createElement, type ComponentPropsWithRef } from "react";
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
  ...props
}: (
  | (ComponentPropsWithRef<"div"> & { as?: "div" })
  | (ComponentPropsWithRef<"span"> & { as: "span" })
) & {
  label: string;
  styles?: stylex.StyleXArray<stylex.StyleXStyles | null | false>;
}) {
  const styled = stylex.props(styles.region, extra);
  return createElement(
    Tag,
    {
      ...props,
      role: "region",
      "aria-label": label,
      tabIndex: props.tabIndex ?? 0,
      ...styled,
      className: cn(styled.className, className),
    },
    children,
  );
}
