import * as stylex from "@stylexjs/stylex";
import { color, layout, motion, radius, space, surface } from "./tokens.stylex";

export const FIELD_SURFACE = stylex.create({
  face: {
    appearance: "none",
    borderRadius: radius.field,
    borderWidth: "var(--control-edge-width)",
    borderStyle: "solid",
    borderColor: {
      default: "var(--field-border)",
      ":is(:focus, :focus-within, [data-popup-open]):not([aria-invalid=true])":
        surface.fieldFocus,
      ":is([aria-invalid=true])": color.negative,
    },
    backgroundColor: "var(--field-background)",
    boxShadow: "var(--field-shadow)",
    transitionProperty: "color, background-color, border-color, outline-color",
    transitionDuration: motion.color,
    transitionTimingFunction: motion.easeState,
  },
  trigger: {
    color: {
      default: color.fgSoft,
      ":hover:not(:disabled, [aria-disabled=true], [data-disabled])": color.fg,
      ":is(:focus-within, [data-popup-open])": color.fg,
    },
    opacity: {
      default: 1,
      ':is(:disabled, [aria-disabled="true"], [data-disabled])':
        "var(--control-disabled-opacity)",
    },
  },
});

export const MENU_SURFACE = stylex.create({
  content: { padding: layout.menuInset, scrollPaddingBlock: layout.menuInset },
  item: {
    minHeight: "max(var(--menu-row-height), var(--control-target-min))",
    gap: layout.controlGap,
    borderRadius: radius.menu,
    borderWidth: 0,
    paddingInline: space.s2,
    paddingBlock: space.s1,
    color: color.fg,
    cursor: "default",
    textAlign: "start",
    overflowWrap: "anywhere",
    backgroundColor: {
      default: "transparent",
      ":hover:not(:disabled, [aria-disabled=true], [data-disabled])": {
        "@media (hover: hover)": surface.hover,
      },
      ":is([data-highlighted]):not(:disabled, [aria-disabled=true], [data-disabled])":
        surface.hover,
      ':is([aria-selected="true"])': surface.selected,
      ':is([aria-selected="true"]):is(:hover, [data-highlighted]):not(:disabled, [aria-disabled=true], [data-disabled])':
        surface.selectedHover,
    },
    opacity: {
      default: 1,
      ':is(:disabled, [aria-disabled="true"], [data-disabled])':
        "var(--control-disabled-opacity)",
    },
    transitionProperty: "color, background-color",
    transitionDuration: motion.color,
    transitionTimingFunction: motion.easeState,
  },
});

export const CARD_SURFACE = stylex.create({
  face: {
    borderRadius: radius.card,
    backgroundColor: surface.card,
    borderWidth: "var(--control-edge-width)",
    borderStyle: "solid",
    borderColor: "transparent",
    boxShadow: "var(--shadow-surface-card)",
  },
});

export const CHOICE_SURFACE = stylex.create({
  mark: {
    display: "grid",
    height: "var(--control-mark-size)",
    width: "var(--control-mark-size)",
    flexShrink: 0,
    placeItems: "center",
    borderWidth: "var(--control-edge-width)",
    borderStyle: "solid",
    borderColor: surface.controlEdge,
    backgroundColor: surface.canvas,
    lineHeight: 1,
    color: color.fgMuted,
  },
  square: { borderRadius: radius.mark },
  selected: {
    borderColor: color.accent,
    backgroundColor: color.accent,
    color: color.onAccent,
  },
});

export const FIELD_SIZE = stylex.create({
  sm: {
    minHeight: "max(var(--control-height-sm), var(--control-target-min))",
    paddingInline: space.s2,
  },
  md: {
    minHeight: "max(var(--control-height-md), var(--control-target-min))",
    paddingInline: space.s2_5,
  },
  lg: {
    minHeight: "max(var(--control-height-lg), var(--control-target-min))",
    paddingInline: space.s3,
  },
});
