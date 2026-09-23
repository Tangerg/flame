import { useSyncExternalStore } from "react";

export type Scheme = "dark" | "light";

export interface VisualStyleMotion {
  instantMs: number;
  fastMs: number;
  mediumMs: number;
  slowMs: number;
  drawerMs: number;
  easeOut: readonly [number, number, number, number];
  drawerProgress: readonly [number, number, ...number[]];
  pressScale: number;
}

export const DEFAULT_MOTION: VisualStyleMotion = {
  instantMs: 80,
  fastMs: 150,
  mediumMs: 200,
  slowMs: 300,
  drawerMs: 500,
  easeOut: [0.22, 1, 0.36, 1],
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

export function useTokenRevision(): object {
  return useSyncExternalStore(subscribe, () => tokenRevision);
}
