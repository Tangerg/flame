import type { VisualStyleSpec } from "@/plugins/sdk";
import { DEFAULT_MOTION } from "@/lib/appearance";
import { visualStyleTokens } from "./tokens";

/**
 * The style says WHERE the depth is, the theme says HOW DARK: colour stays semantic here so
 * every colour theme inherits the same region algorithm with no hard-coded light or dark.
 */
export const flameStyle: VisualStyleSpec = {
  id: "flame",
  motion: DEFAULT_MOTION,
  tokens: visualStyleTokens({}),
};
