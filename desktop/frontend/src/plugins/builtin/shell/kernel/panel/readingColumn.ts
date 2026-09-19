import * as stylex from "@stylexjs/stylex";

export const readingColumn = stylex.create({
  box: { marginInline: "auto", width: "100%", maxWidth: "var(--reading-column-max)" },
  // GUTTER is applied by whatever draws INSIDE the box — a message, the composer, a banner —
  // never by the scroller's content wrapper. On the wrapper it is wrong twice: the composer
  // wears the box without it, so read text comes out narrower than typed text, and each
  // message's box then hugs its text, where paint containment slices the action bar's inset.
  gutter: {
    paddingInline: {
      default: "var(--reading-gutter)",
      "@container conversation (min-width: 640px)": "var(--reading-gutter-wide)",
    },
  },
  // A child contributes to the observed content box; padding on that box does not.
  clearance: { height: "calc(var(--composer-overlay, 0px) + 1rem)", flexShrink: 0 },
});

/** The property the composer measures itself into, read by `clearance` above. Kept beside it
 *  because they are two halves of one contract: a name written apart drifts into a last
 *  message nobody can scroll out from under. */
export const COMPOSER_OVERLAY_PROPERTY = "--composer-overlay";
