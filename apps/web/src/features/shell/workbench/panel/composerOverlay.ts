import * as stylex from "@stylexjs/stylex";

export const COMPOSER_OVERLAY_PROPERTY = "--composer-overlay";

export const composerOverlay = stylex.create({
  clearance: { height: "calc(var(--composer-overlay, 0px) + 1rem)", flexShrink: 0 },
});
