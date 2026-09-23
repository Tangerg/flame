import * as stylex from "@stylexjs/stylex";
import { color, motion, radius, space, surface } from "@/styles/tokens.stylex";

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
    transitionProperty: "opacity, scale, translate",
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
  options: { borderRadius: radius.lg },
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
    boxShadow: "var(--shadow-modal)",
  },
  modalCentred: { inset: 0, margin: "auto", height: "fit-content" },
  modalTop: { insetInline: 0, top: "calc(var(--spacing) * 24)", marginInline: "auto" },
});

export const formDialog = stylex.create({
  plane: { borderRadius: radius.floatingPanel, backgroundColor: surface.card },
  inset: { padding: space.s5 },
  actions: {
    marginTop: space.s4,
    display: "flex",
    alignItems: "center",
    justifyContent: "flex-end",
    gap: space.s2,
  },
});

export const FLOATING_LAYER = [styles.layer];

export const FLOATING_PANEL = [styles.face, styles.motion, styles.panel];
export const FLOATING_OPTIONS = [styles.face, styles.motion, styles.options];
export const FLOATING_TIP = [styles.face, styles.motion, styles.tip];

export const MODAL_SCRIM = [styles.scrim];

export const modalPanel = (place: "centred" | "top" = "centred") => [
  styles.modal,
  place === "top" ? styles.modalTop : styles.modalCentred,
  styles.motion,
];
