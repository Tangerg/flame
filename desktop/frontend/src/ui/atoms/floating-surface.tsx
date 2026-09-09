import * as stylex from "@stylexjs/stylex";
import { color, motion, radius, surface } from "@/styles/tokens.stylex";

/**
 * The material everything that leaves the document flow is made of.
 *
 * One fill, one cast, one blur, one way of arriving and leaving — so a menu, a popover, a tip
 * and a dialog cannot each decide what floating looks like. These are exported rather than
 * wrapped in components because the primitives that need them are Base UI parts: a `Popup` takes
 * a class, not a surface.
 *
 * These are style exports and not a component on purpose. A component here once rendered a bare
 * `<div>` wearing this material, which promised the name and delivered none of it: no portal, so
 * an `overflow: hidden` ancestor clipped it away, and no positioner, so each call site invented
 * its own coordinates. Leaving the flow is the Popover's and the Menu's job — Base UI's Portal
 * and Positioner — and this file only says what the thing that left is made of.
 */
const styles = stylex.create({
  // The blur lives on a pseudo-element behind the fill rather than on the surface itself: a
  // `backdrop-filter` on the element would be clipped by its own `overflow: hidden`, and an
  // element that filters its backdrop also becomes a containing block for its descendants.
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
  // Arriving and leaving are the same gesture at two speeds: a panel rises into place, and
  // dismisses faster than it appeared because a dismissal is already decided.
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
  // What the scrim sits in front of. Four dialogs had each written this out, so the four agreed
  // on nothing but these: a modal is fixed, on the modal layer, and cast by `--shadow-modal`.
  modal: {
    position: "fixed",
    zIndex: "var(--layer-modal)",
    boxShadow: "var(--shadow-modal)",
  },
  // Centred is the standing answer; a finder anchors near the top because the list below it
  // grows downward and a centred one would walk up the screen as results arrive.
  modalCentred: { inset: 0, margin: "auto", height: "fit-content" },
  modalTop: { insetInline: 0, top: "calc(var(--spacing) * 24)", marginInline: "auto" },
});

/** Where a floating thing sits in the stack. Its own layer, not the modal one. */
export const FLOATING_LAYER = [styles.layer];

/** The panel: a menu, a popover, a suggestion list. */
export const FLOATING_PANEL = [styles.face, styles.motion, styles.panel];

/** The tip: the same material at the smaller corner a label deserves. */
export const FLOATING_TIP = [styles.face, styles.motion, styles.tip];

/** What a modal puts between itself and everything behind it. */
export const MODAL_SCRIM = [styles.scrim];

/** The panel the scrim sits in front of. Width, fill and corner stay with each dialog. */
export const modalPanel = (place: "centred" | "top" = "centred") => [
  styles.modal,
  place === "top" ? styles.modalTop : styles.modalCentred,
  styles.motion,
];
