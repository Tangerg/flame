import type { VisualStyleSpec } from "@/plugins/sdk";
import { visualStyleTokens, WORKBENCH_MOTION } from "./tokens";

/**
 * The style says WHERE the depth is, the theme says HOW DARK: colour stays semantic here so
 * every colour theme inherits the same region algorithm with no hard-coded light or dark.
 */
export const flameStyle: VisualStyleSpec = {
  id: "flame",
  label: "Flame Workbench",
  description: "Tool-window geometry: opaque columns, borderless cards, half-pixel seams.",
  order: -10,
  traits: { regions: "tool-windows", controls: "quiet" },
  motion: WORKBENCH_MOTION,
  preview: {
    canvas: "#1d1f23",
    sidebar: "#2a2d32",
    dock: "#24272c",
    edge: "#3a3d42",
    accent: "#3574f0",
  },
  tokens: visualStyleTokens({}),
};
