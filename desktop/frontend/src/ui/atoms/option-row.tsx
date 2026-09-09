import * as stylex from "@stylexjs/stylex";
import { cn } from "@/lib/classNames";
import { color, motion, radius, space, surface, type } from "@/styles/tokens.stylex";
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
    transitionTimingFunction: "var(--ease-out)",
  },
  grid: { display: "grid" },
  flex: { display: "flex" },
  glyph: { display: "grid", gridTemplateColumns: "auto minmax(0, 1fr)" },
  // A row in a one-of list: what it is, its name, and whether it is the one. Six call sites had
  // each written the template — the mark column in two widths and the glyph in three — so the
  // labels did not start on one line between two menus and the checks did not either. The glyph
  // column is a channel because a swatch is wider than an icon; everything else is fixed.
  pick: {
    display: "grid",
    gridTemplateColumns:
      "var(--menu-glyph, calc(var(--spacing) * 4)) minmax(0, 1fr) calc(var(--spacing) * 3.5)",
  },
  /** The same row where the option needs no glyph: a language, a font. */
  pickPlain: {
    display: "grid",
    gridTemplateColumns: "minmax(0, 1fr) calc(var(--spacing) * 3.5)",
  },
  /** The same row where the glyph is a swatch rather than an icon, and so is wider. */
  pickWide: { "--menu-glyph": "calc(var(--spacing) * 6)" },
  sm: { minHeight: "var(--menu-row-height)", paddingBlock: "1px" },
  md: { height: "calc(var(--spacing) * 8)" },
  lg: { minHeight: "calc(var(--spacing) * 9)", paddingBlock: space.s1_5 },
  // A row that destroys something says so before it is chosen, and keeps saying it once it is.
  destructive: {
    color: { default: color.negative, ":is([data-highlighted])": color.negative },
    backgroundColor: { default: null, ":is([data-highlighted])": surface.negativeWash },
  },
});

export const floatingRow = (layout: RowLayout = "grid", size: RowSize = "md") => [
  floatingRowStyles.base,
  type.uiMd,
  // `pickWide` is `pick` with a wider glyph column, so it composes both rather than repeating
  // the template — a second copy is how the six spellings started.
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
