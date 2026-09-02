// A passive snapshot the painter publishes into. Reads no store, registry or DOM; the
// defaults apply only before the first publish.

import { useSyncExternalStore } from "react";

// Declared here, not in the theme context that owns the rest of the appearance vocabulary,
// because these two travel DOWN this seam: `lib/highlight` reads the scheme and `lib/motion`
// reads the motion, and neither may import a plugin.
export type Scheme = "dark" | "light";

export interface VisualStyleMotion {
  instantMs: number;
  fastMs: number;
  mediumMs: number;
  slowMs: number;
  drawerMs: number;
  easeOut: readonly [number, number, number, number];
  /** Evenly spaced samples of the structural-panel spring, published as CSS `linear()`. */
  drawerProgress: readonly [number, number, ...number[]];
  pressScale: number;
}

/** The shipped style's motion, and the snapshot's value until that style publishes it. One
 *  copy: a fallback that disagrees with the style is one nobody notices is wrong. */
export const DEFAULT_MOTION: VisualStyleMotion = {
  instantMs: 80,
  fastMs: 150,
  mediumMs: 200,
  slowMs: 300,
  drawerMs: 500,
  easeOut: [0.22, 1, 0.36, 1],
  // A sampled spring as native `linear()`, not a fitted cubic: it keeps the overshoot, and
  // native reversal keeps an interrupted gesture continuous with no React frame owner.
  drawerProgress: [
    0, 0.06981, 0.21761, 0.38345, 0.53716, 0.66615, 0.76765, 0.84375, 0.89859, 0.93672, 0.96233,
    0.97894, 0.98929, 0.99544, 0.99887, 1.00061, 1.00135, 1.00152, 1.00142, 1.00119, 1,
  ],
  pressScale: 0.98,
};

let scheme: Scheme = "dark";
let scale = 1;
let motion = DEFAULT_MOTION;
let tokenRevision: object = {};
const listeners = new Set<() => void>();

function announce(): void {
  for (const listener of listeners) listener();
}

export function publishScheme(next: Scheme): void {
  if (next === scheme) return;
  scheme = next;
  announce();
}

/** Signals that `:root`'s colour tokens were rewritten, for code that must read COMPUTED
 *  values (an SVG generator, a canvas) and so needs to know when to re-read. */
export function publishTokens(): void {
  tokenRevision = {};
  announce();
}

export function publishMotionScale(next: number): void {
  if (scale === next) return;
  scale = next;
  announce();
}

export function publishVisualStyleMotion(next: VisualStyleMotion): void {
  motion = next;
}

export function motionScale(): number {
  return scale;
}

export function visualStyleMotion(): VisualStyleMotion {
  return motion;
}

function subscribe(onChange: () => void): () => void {
  listeners.add(onChange);
  return () => listeners.delete(onChange);
}

function snapshot(): Scheme {
  return scheme;
}

export function useScheme(): Scheme {
  return useSyncExternalStore(subscribe, snapshot);
}

/** An opaque, monotonic stamp of the last token repaint — an invalidation key for
 *  anything that reads computed token values. */
export function useTokenRevision(): object {
  return useSyncExternalStore(subscribe, () => tokenRevision);
}
