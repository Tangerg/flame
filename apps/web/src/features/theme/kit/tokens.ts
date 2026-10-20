import { type Scheme } from "@/lib/appearance";
import { DEFAULT_CONTRAST } from "@/features/theme/kit/appearance";
export const SCHEME_ICON: Record<Scheme, string> = {
  dark: "moon",
  light: "sun",
};

export function depthStep(scheme: Scheme, contrast: number): string {
  const step = (2 + (contrast / 100) * 8) * (scheme === "dark" ? 2 : 1);
  return `${Number(step.toFixed(1))}%`;
}

const INK_TRACKING = 3;

export function inkTracksLadder(scheme: Scheme, ink: string): string {
  const rest = depthStep(scheme, DEFAULT_CONTRAST);
  return `color-mix(in oklab, var(--color-text) max(0%, calc((var(--depth-step) - ${rest}) * ${INK_TRACKING})), ${ink})`;
}
