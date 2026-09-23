import * as stylex from "@stylexjs/stylex";
import { cn } from "@/lib/classNames";
import { color, motion, radius, space, surface, type } from "@/styles/tokens.stylex";
import { Pressable, type PressableProps } from "./pressable";

export type RowLayout = "grid" | "flex" | "glyph" | "pick" | "pickWide" | "pickPlain";
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
      ':is([aria-selected="true"]):is(:hover, [data-highlighted])': surface.selectedHover,
      ":is([data-highlighted])": surface.hover,
    },
    paddingInline: space.s2,
    textAlign: "left",
    color: color.fg,
    transitionProperty: "color, background-color",
    transitionDuration: motion.color,
    transitionTimingFunction: motion.easeState,
  },
  grid: { display: "grid" },
  flex: { display: "flex" },
  glyph: { display: "grid", gridTemplateColumns: "auto minmax(0, 1fr)" },
  pick: {
    display: "grid",
    gridTemplateColumns:
      "var(--menu-glyph, calc(var(--spacing) * 4)) minmax(0, 1fr) calc(var(--spacing) * 3.5)",
  },
  pickPlain: {
    display: "grid",
    gridTemplateColumns: "minmax(0, 1fr) calc(var(--spacing) * 3.5)",
  },
  pickWide: { "--menu-glyph": "calc(var(--spacing) * 6)" },
  sm: { minHeight: "var(--menu-row-height)", paddingBlock: "1px" },
  md: { height: "calc(var(--spacing) * 8)" },
  lg: { minHeight: "calc(var(--spacing) * 9)", paddingBlock: space.s1_5 },
  destructive: {
    color: { default: color.negative, ":is([data-highlighted])": color.negative },
    backgroundColor: { default: null, ":is([data-highlighted])": surface.negativeWash },
  },
});

export const floatingRow = (layout: RowLayout = "grid", size: RowSize = "md") => [
  floatingRowStyles.base,
  type.uiMd,
  layout === "pickWide" ? floatingRowStyles.pick : floatingRowStyles[layout],
  layout === "pickWide" && floatingRowStyles.pickWide,
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
