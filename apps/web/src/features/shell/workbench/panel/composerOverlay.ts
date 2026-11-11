import * as stylex from "@stylexjs/stylex";

export const COMPOSER_OVERLAY_PROPERTY = "--composer-overlay";

export const composerOverlay = stylex.create({
  clearance: { height: "var(--transcript-bottom-clearance)", flexShrink: 0 },
});
