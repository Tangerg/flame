import * as stylex from "@stylexjs/stylex";
import { cn } from "@/lib/classNames";
import { color, radius, space, surface, type } from "@/styles/tokens.stylex";
import { Pressable, type PressableProps } from "./pressable";

/**
 * A row inside something floating: a menu item, a suggestion, a search result.
 *
 * `glyph` exists because `grid` only ever gave `display: grid`, and a grid with no template is
 * one column — so all three call sites that wanted a glyph beside a label had to supply the
 * template themselves, in two spellings. That is not customisation; the step was half a step.
 *
 * `minmax(0, 1fr)` rather than `1fr`: both behave the same here, because every second child
 * truncates and an `overflow: hidden` item already has an automatic minimum of zero. The
 * explicit form says so instead of depending on it.
 */
export type RowLayout = "grid" | "flex" | "glyph";
type RowSize = "sm" | "md" | "lg";

export const floatingRowStyles = stylex.create({
  base: {
    width: "100%",
    alignItems: "center",
    gap: space.s2,
    borderRadius: radius.sm,
    borderWidth: 0,
    backgroundColor: {
      default: "transparent",
      ":hover": surface.hover,
      ':is([aria-selected="true"])': surface.selected,
      ":is([data-highlighted])": surface.hover,
    },
    paddingInline: space.s2,
    textAlign: "left",
    color: color.fg,
    outline: "none",
    transitionProperty: "color, background-color",
    transitionDuration: "0.15s",
    transitionTimingFunction: "cubic-bezier(0.4, 0, 0.2, 1)",
  },
  grid: { display: "grid" },
  flex: { display: "flex" },
  glyph: { display: "grid", gridTemplateColumns: "auto minmax(0, 1fr)" },
  sm: { minHeight: "var(--menu-row-height)", paddingBlock: "1px" },
  md: { height: "calc(var(--spacing) * 8)" },
  lg: { minHeight: "calc(var(--spacing) * 9)", paddingBlock: space.s1_5 },
  // A row that destroys something says so before it is chosen, and keeps saying it once it is.
  destructive: {
    color: { default: color.negative, ":is([data-highlighted])": color.negative },
    backgroundColor: { default: null, ":is([data-highlighted])": surface.negativeWashRow },
  },
});

export const floatingRow = (layout: RowLayout = "grid", size: RowSize = "md") => [
  floatingRowStyles.base,
  type.uiMd,
  floatingRowStyles[layout],
  floatingRowStyles[size],
];

export type OptionRowProps = Omit<PressableProps, "aria-selected"> & {
  layout?: RowLayout;
  size?: RowSize;
  selected?: boolean;
};

export function OptionRow({ layout, size, selected, className, ...props }: OptionRowProps) {
  const styled = stylex.props(floatingRow(layout, size));
  return (
    <Pressable
      {...props}
      type={props.type ?? "button"}
      {...(selected === undefined
        ? {}
        : { role: props.role ?? "option", "aria-selected": selected })}
      {...styled}
      className={cn(styled.className, className)}
    />
  );
}
