import {
  createContext,
  createElement,
  useContext,
  useSyncExternalStore,
  type ReactNode,
} from "react";
import { normalizeUiFontSize, UI_FONT_SIZE_DEFAULT_PX } from "@flame/ui/typography";
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
  pressScale: 0.96,
};

export interface AppearanceSnapshot {
  scheme: Scheme;
  motionScale: number;
  motion: VisualStyleMotion;
  tokenRevision: object;
  uiFontSize: number;
}

/** The current document paint, observed without owning its preferences. */
export interface AppearanceProjection {
  read(): AppearanceSnapshot;
  subscribe(listener: () => void): () => void;
}

export function createAppearanceProjection() {
  let current: AppearanceSnapshot = {
    scheme: "dark",
    motionScale: 1,
    motion: DEFAULT_MOTION,
    tokenRevision: {},
    uiFontSize: UI_FONT_SIZE_DEFAULT_PX,
  };
  const listeners = new Set<() => void>();
  function update(patch: Partial<AppearanceSnapshot>) {
    current = { ...current, ...patch };
    for (const listener of listeners) listener();
  }
  return {
    read: () => current,
    subscribe(listener: () => void) {
      listeners.add(listener);
      return () => {
        listeners.delete(listener);
      };
    },
    publishScheme(next: Scheme) {
      if (next !== current.scheme) update({ scheme: next });
    },
    publishTokens() {
      update({ tokenRevision: {} });
    },
    publishUiFontSize(value: number | null) {
      const next = normalizeUiFontSize(value);
      if (next !== current.uiFontSize) update({ uiFontSize: next });
    },
    publishMotionScale(next: number) {
      if (next !== current.motionScale) update({ motionScale: next });
    },
    publishVisualStyleMotion(next: VisualStyleMotion) {
      if (next !== current.motion) update({ motion: next });
    },
  };
}

export type AppearancePublisher = ReturnType<typeof createAppearanceProjection>;

const AppearanceContext = createContext<AppearanceProjection | null>(null);

export function AppearanceProvider({
  value,
  children,
}: {
  value: AppearanceProjection;
  children: ReactNode;
}) {
  return createElement(AppearanceContext, { value }, children);
}

export function useAppearanceProjection(): AppearanceProjection {
  const value = useContext(AppearanceContext);
  if (!value) throw new Error("AppearanceProvider is missing");
  return value;
}

export function useScheme(): Scheme {
  const appearance = useAppearanceProjection();
  return useSyncExternalStore(appearance.subscribe, () => appearance.read().scheme);
}

export function useTokenRevision(): object {
  const appearance = useAppearanceProjection();
  return useSyncExternalStore(appearance.subscribe, () => appearance.read().tokenRevision);
}

export function useUiFontSize(): number {
  const appearance = useAppearanceProjection();
  return useSyncExternalStore(appearance.subscribe, () => appearance.read().uiFontSize);
}
