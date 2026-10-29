import { clampChroma, formatHex, interpolate, oklchColor } from "@/lib/color";

export interface Oklch {
  l: number;
  c: number;
  h: number;
}

export function toOklch(color: string): Oklch {
  const { l, c, h } = oklchColor(color)!;
  return { l, c, h: h ?? 0 };
}

export function fromOklch(color: Oklch): string {
  return formatHex(clampChroma({ mode: "oklch", ...color }, "oklch"));
}

export function mixOklab(from: string, to: string, pct: number): string {
  return formatHex(interpolate([to, from], "oklab")(pct / 100));
}
