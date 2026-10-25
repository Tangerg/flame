import { type PaperTexture } from "@/features/theme/kit/appearance";
import { type Scheme } from "@/lib/appearance";
import { colord } from "colord";

const PAPERS = {
  xuan: { frequency: "0.42 0.55", octaves: 3, seed: 7 },
  cotton: { frequency: "0.28 0.32", octaves: 4, seed: 17 },
  grain: { frequency: "0.8", octaves: 3, seed: 29 },
} as const;

const FIBERS =
  "M18 42l11 4 6-1 9 5M82 15q-3 14 9 38M137 68l9-8 12-3 8-9M216 27q18 12 29 36" +
  "M284 16l-2 14 5 11M348 59q16-9 42-3M425 25l-7 12-3 17M475 93q-9 13-5 31" +
  "M29 132l13 3 17-4M102 107q9 18 34 27M171 163l8-17 3-16M252 113q-8 20-3 39" +
  "M312 147l11 4 8 10M380 104q18 7 27 25M436 173l-3-18 4-11M18 221q16-4 38 3" +
  "M92 193l7 11 1 15 6 9M166 240q20-12 35-9M232 206l-8 13-2 16M291 257q17 6 28 21" +
  "M354 204l12 7 15-2M414 241q-4 21 7 36M476 208l-9 15 1 9M44 305q8 22 24 35" +
  "M115 278l18 1 9 6M185 330q-5 15 4 33M244 297l12-4 11 3 8-2M323 346q12-18 28-20" +
  "M395 297l-2 12 7 18M456 359q18 6 28 22M19 400l10 7 7 1 8 7M95 373q11 22 7 42" +
  "M153 437l12-5 18 3M224 387q-6 17 5 35M291 450l8-13 2-12M369 397q18 0 35 13" +
  "M433 454l-2-14 5-14M480 408q-8 17-3 37M57 475l15-3 12 6M192 484q14-7 29-4" +
  "M329 485l8-11 15-6";

function paperOpacity(texture: PaperTexture, strength: number) {
  return { grain: strength * 0.03, fiber: texture === "xuan" ? strength * 0.02 : 0 };
}

export function paperTextureBackgrounds(
  texture: PaperTexture,
  strength: number,
  scheme: Scheme,
  planes: readonly string[],
): string[] {
  if (texture === "off") return [];
  const { grain, fiber } = paperOpacity(texture, strength);
  // Both image layers use the same ink; their combined ceiling bounds text contrast.
  const alpha = 1 - (1 - grain) * (1 - fiber);
  const ink = scheme === "dark" ? 255 : 0;
  return planes.map((plane) => {
    const { r, g, b } = colord(plane).toRgb();
    return colord({
      r: r * (1 - alpha) + ink * alpha,
      g: g * (1 - alpha) + ink * alpha,
      b: b * (1 - alpha) + ink * alpha,
    }).toHex();
  });
}

export function paperTextureImage(texture: PaperTexture, strength: number, scheme: Scheme): string {
  if (texture === "off") return "none";
  const { frequency, octaves, seed } = PAPERS[texture];
  const ink = scheme === "dark" ? 1 : 0;
  const { grain, fiber } = paperOpacity(texture, strength);
  // A fixed, seamless image paints behind content; it never filters the text.
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="512" height="512" viewBox="0 0 512 512"><filter id="paper" x="0" y="0" width="100%" height="100%" color-interpolation-filters="sRGB"><feTurbulence type="fractalNoise" baseFrequency="${frequency}" numOctaves="${octaves}" seed="${seed}" stitchTiles="stitch"/><feColorMatrix type="matrix" values="0 0 0 0 ${ink} 0 0 0 0 ${ink} 0 0 0 0 ${ink} -3 0 0 0 1.9"/></filter><rect width="100%" height="100%" opacity="${grain}" filter="url(#paper)"/>${fiber ? `<path d="${FIBERS}" fill="none" stroke="${ink ? "white" : "black"}" stroke-width="0.45" stroke-linecap="round" opacity="${fiber}"/>` : ""}</svg>`;
  return `url("data:image/svg+xml,${encodeURIComponent(svg)}")`;
}
