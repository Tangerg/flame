// A PASSIVE snapshot: leaf modules read the active appearance without reaching into the
// preference store or plugin registry, which would invert the dependency ring from leaf UI
// into application composition. The theme context publishes into it from the painter.
// Nothing here reads a store, a registry, or the DOM.
//
// The defaults apply only before the first publish, and in tests that install no painter.

import { useSyncExternalStore } from "react";

// Declared here because this is the ONE module every ring may import, so the design system,
// the SDK theme contract and the theme kit cannot spell the union out separately.
export type Scheme = "dark" | "light";
export type ColorThemeId = string;
export type VisualStyleId = string;

// Lives here rather than beside the maths in `theme/kit/accentTint` because the preference
// store persists it and `state` sits BELOW the plugins.
export const ACCENT_TINTS = ["off", "soft", "standard"] as const;
export type AccentTint = (typeof ACCENT_TINTS)[number];

export const DEFAULT_ACCENT_TINT: AccentTint = "standard";

export interface VisualStyleMotion {
  instantMs: number;
  fastMs: number;
  mediumMs: number;
  disclosureMs: number;
  slowMs: number;
  drawerMs: number;
  easeOut: readonly [number, number, number, number];
  easeInOut: readonly [number, number, number, number];
  easeEmphasized: readonly [number, number, number, number];
  /** Evenly spaced samples of the structural-panel spring, published as CSS `linear()`. */
  drawerProgress: readonly [number, number, ...number[]];
  pressScale: number;
}

/** Every value MUST match the shipped style (WORKBENCH_MOTION): a fallback that disagrees
 *  is a fallback nobody notices is wrong. */
const DEFAULT_MOTION: VisualStyleMotion = {
  instantMs: 80,
  fastMs: 150,
  mediumMs: 200,
  disclosureMs: 220,
  slowMs: 360,
  drawerMs: 500,
  easeOut: [0.22, 1, 0.36, 1],
  easeInOut: [0.45, 0, 0.55, 1],
  easeEmphasized: [0.16, 1, 0.3, 1],
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

/**
 * Publish that the colour tokens on `:root` were just rewritten.
 *
 * For the code that can't use a token — an SVG generator handed literal colours,
 * a canvas — and has to read the computed values instead. It needs to know WHEN
 * to re-read, and only the painter knows that. Consumers subscribe to appearance
 * replacement rather than guessing which preferences affect computed colours.
 */
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
