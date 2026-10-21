import * as stylex from "@stylexjs/stylex";
import { color, layout, motion, radius, space, surface } from "./tokens.stylex";

const MODAL_TOP_INSET = space.s24;

const styles = stylex.create({
  face: {
    position: "relative",
    isolation: "isolate",
    overflow: "hidden",
    color: color.fg,
    backgroundColor: surface.floating,
    boxShadow: "var(--shadow-popover)",
    "::before": {
      content: "",
      pointerEvents: "none",
      position: "absolute",
      inset: 0,
      zIndex: -1,
      borderRadius: "inherit",
      backdropFilter: "var(--floating-backdrop)",
    },
  },
  motion: {
    transitionProperty: {
      default: "opacity, scale, translate",
      ':is([data-instant="dismiss"])': "none",
    },
    transitionTimingFunction: "var(--ease-out)",
    transitionDuration: {
      default: motion.fast,
      ":is([data-ending-style])": motion.instant,
    },
    scale: {
      default: null,
      ":is([data-starting-style])": 0.97,
      ":is([data-ending-style])": 0.97,
    },
    translate: {
      default: null,
      ":is([data-starting-style])": "0 calc(var(--spacing) * 1)",
      ":is([data-ending-style])": "0 calc(var(--spacing) * 1)",
    },
    opacity: {
      default: null,
      ":is([data-starting-style])": 0,
      ":is([data-ending-style])": 0,
    },
  },
  panel: { borderRadius: radius.floatingPanel },

  tip: { borderRadius: radius.floatingTip },
  layer: { zIndex: "var(--layer-floating)" },
  scrim: {
    position: "fixed",
    inset: 0,
    zIndex: "var(--layer-modal)",
    backgroundColor: surface.scrim,
    transitionProperty: "opacity",
    transitionTimingFunction: "var(--ease-out)",
    transitionDuration: {
      default: motion.fast,
      ":is([data-ending-style])": motion.instant,
    },
    opacity: {
      default: null,
      ":is([data-starting-style])": 0,
      ":is([data-ending-style])": 0,
    },
  },
  rise: { animation: motion.riseIn },
  modal: {
    position: "fixed",
    zIndex: "var(--layer-modal)",
    borderRadius: radius.floatingPanel,
    backgroundColor: surface.card,
    color: color.fg,
    boxShadow: "var(--shadow-modal)",
    maxHeight: "calc(100dvh - var(--layout-compact-gutter) * 2)",
    overflowY: "auto",
    overscrollBehavior: "contain",
    scrollPaddingBlock: layout.compactGutter,
  },
  modalCentred: { inset: 0, margin: "auto", height: "fit-content" },
  modalTop: {
    insetInline: 0,
    top: MODAL_TOP_INSET,
    maxHeight: `calc(100dvh - ${MODAL_TOP_INSET} - ${layout.compactGutter})`,
    marginInline: "auto",
  },
});

export const FLOATING_LAYER = [styles.layer];

export const FLOATING_PANEL = [styles.face, styles.motion, styles.panel];
export const FLOATING_TIP = [styles.face, styles.motion, styles.tip];

export const MODAL_SCRIM = [styles.scrim];

export const modalPanel = (place: "centred" | "top" = "centred") => [
  styles.modal,
  place === "top" ? styles.modalTop : styles.modalCentred,
  styles.motion,
];
