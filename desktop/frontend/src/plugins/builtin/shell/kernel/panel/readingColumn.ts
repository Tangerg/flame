import * as stylex from "@stylexjs/stylex";

export const readingColumn = stylex.create({
  box: { marginInline: "auto", width: "100%", maxWidth: "var(--reading-column-max)" },
  gutter: {
    paddingInline: {
      default: "var(--reading-gutter)",
      "@container conversation (min-width: 640px)": "var(--reading-gutter-wide)",
    },
  },
  clearance: { height: "calc(var(--composer-overlay, 0px) + 1rem)", flexShrink: 0 },
});

export const COMPOSER_OVERLAY_PROPERTY = "--composer-overlay";
