import { type Scheme } from "@/lib/appearance";
import { DEFAULT_CONTRAST } from "@/features/theme/kit/appearance";
export const SCHEME_ICON: Record<Scheme, string> = {
  dark: "moon",
  light: "sun",
};

function depthPercent(scheme: Scheme, contrast: number): number {
  const step = (2 + (contrast / 100) * 8) * (scheme === "dark" ? 2 : 1);
  return Number(step.toFixed(1));
}

export function depthStep(scheme: Scheme, contrast: number): string {
  return `${depthPercent(scheme, contrast)}%`;
}

const INK_TRACKING = 3;

export function inkTracksLadder(scheme: Scheme, ink: string): string {
  const rest = depthStep(scheme, DEFAULT_CONTRAST);
  return `color-mix(in oklab, var(--color-text) max(0%, calc((var(--depth-step) - ${rest}) * ${INK_TRACKING})), ${ink})`;
}

// The percentage inkTracksLadder resolves to once --depth-step is depthStep(scheme, contrast).
export function inkTrackingMix(scheme: Scheme, contrast: number): number {
  const delta = depthPercent(scheme, contrast) - depthPercent(scheme, DEFAULT_CONTRAST);
  return Math.max(0, delta * INK_TRACKING);
}
