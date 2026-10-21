import * as stylex from "@stylexjs/stylex";
import { color, surface } from "./tokens.stylex";

export const toneFill = stylex.create({
  neutral: { backgroundColor: surface.surface2, color: color.fgMuted },
  accent: {
    backgroundColor: surface.accentBadge,
    color: color.accentBadgeText,
  },
  success: {
    backgroundColor: surface.successBadge,
    color: color.successBadgeText,
  },
  warning: {
    backgroundColor: surface.warningBadge,
    color: color.warningBadgeText,
  },
  negative: {
    backgroundColor: surface.negativeBadge,
    color: color.negativeBadgeText,
  },
  info: { backgroundColor: surface.infoBadge, color: color.infoBadgeText },
});
