import type { VisualStyleSpec } from "@/plugins/sdk";
import { DEFAULT_MOTION } from "@/lib/appearance";
import { visualStyleTokens } from "./tokens";

export const flameStyle: VisualStyleSpec = {
  id: "flame",
  motion: DEFAULT_MOTION,
  tokens: visualStyleTokens({}),
};
