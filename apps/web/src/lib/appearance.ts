import {
  createContext,
  createElement,
  useContext,
  useSyncExternalStore,
  type ReactNode,
} from "react";
import { normalizeUiFontSize, UI_FONT_SIZE_DEFAULT_PX } from "@flame/ui/typography";
import { DEFAULT_MOTION, type VisualStyleMotion } from "@/lib/visualStyleMotion";
export type Scheme = "dark" | "light";

export interface AppearanceSnapshot {
  scheme: Scheme;
  motionScale: number;
  motion: VisualStyleMotion;
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

export function useUiFontSize(): number {
  const appearance = useAppearanceProjection();
  return useSyncExternalStore(appearance.subscribe, () => appearance.read().uiFontSize);
}
