import { colord } from "colord";
type Oklab = [number, number, number];

export interface Oklch {
  l: number;
  c: number;
  h: number;
}

const toLinear = (value: number) => {
  const scaled = value / 255;
  return scaled <= 0.04045 ? scaled / 12.92 : ((scaled + 0.055) / 1.055) ** 2.4;
};

function toOklab(color: string): Oklab {
  const { r, g, b } = colord(color).toRgb();
  const [lr, lg, lb] = [toLinear(r), toLinear(g), toLinear(b)];
  const l = Math.cbrt(0.4122214708 * lr + 0.5363325363 * lg + 0.0514459929 * lb);
  const m = Math.cbrt(0.2119034982 * lr + 0.6806995451 * lg + 0.1073969566 * lb);
  const s = Math.cbrt(0.0883024619 * lr + 0.2817188376 * lg + 0.6299787005 * lb);
  return [
    0.2104542553 * l + 0.793617785 * m - 0.0040720468 * s,
    1.9779984951 * l - 2.428592205 * m + 0.4505937099 * s,
    0.0259040371 * l + 0.7827717662 * m - 0.808675766 * s,
  ];
}

function linearRgb([L, a, b]: Oklab): [number, number, number] {
  const l = (L + 0.3963377774 * a + 0.2158037573 * b) ** 3;
  const m = (L - 0.1055613458 * a - 0.0638541728 * b) ** 3;
  const s = (L - 0.0894841775 * a - 1.291485548 * b) ** 3;
  return [
    4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s,
    -1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s,
    -0.0041960863 * l - 0.7034186147 * m + 1.707614701 * s,
  ];
}

function fromOklab(lab: Oklab): string {
  const [r, g, b] = linearRgb(lab).map((value) => {
    const clamped = Math.min(1, Math.max(0, value));
    const encoded = clamped <= 0.0031308 ? clamped * 12.92 : 1.055 * clamped ** (1 / 2.4) - 0.055;
    return Math.round(encoded * 255);
  }) as [number, number, number];
  return colord({ r, g, b }).toHex();
}

const inGamut = (lab: Oklab) =>
  linearRgb(lab).every((value) => value >= -1e-4 && value <= 1 + 1e-4);

export function toOklch(color: string): Oklch {
  const [l, a, b] = toOklab(color);
  return { l, c: Math.hypot(a, b), h: ((Math.atan2(b, a) * 180) / Math.PI + 360) % 360 };
}

export function fromOklch({ l, c, h }: Oklch): string {
  const angle = (h * Math.PI) / 180;
  const at = (chroma: number): Oklab => [l, chroma * Math.cos(angle), chroma * Math.sin(angle)];
  if (inGamut(at(c))) return fromOklab(at(c));
  let low = 0;
  let high = c;
  for (let i = 0; i < 20; i += 1) {
    const mid = (low + high) / 2;
    if (inGamut(at(mid))) low = mid;
    else high = mid;
  }
  return fromOklab(at(low));
}

export function mixOklab(from: string, to: string, pct: number): string {
  const [one, two] = [toOklab(from), toOklab(to)];
  const weight = pct / 100;
  return fromOklab([
    one[0] * weight + two[0] * (1 - weight),
    one[1] * weight + two[1] * (1 - weight),
    one[2] * weight + two[2] * (1 - weight),
  ]);
}
