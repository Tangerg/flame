import { useSyncExternalStore } from "react";
import {
  applyPatch,
  EMPTY_LOCATION,
  sameLocation,
  type AppLocation,
  type LocationPatch,
  type Navigator,
} from "./navigation";

export interface MemoryNavigator extends Navigator {
  reset(location?: Partial<AppLocation>): void;
  entries(): AppLocation[];
}

export function createMemoryNavigator(initial: Partial<AppLocation> = {}): MemoryNavigator {
  let history: AppLocation[] = [applyPatch(EMPTY_LOCATION, initial)];
  let index = 0;
  const listeners = new Set<(location: AppLocation, previous: AppLocation) => void>();

  const current = (): AppLocation => history[index]!;

  const commit = (next: AppLocation, replace: boolean): void => {
    const previous = current();
    if (sameLocation(previous, next)) return;
    if (replace) history[index] = next;
    else {
      history = [...history.slice(0, index + 1), next];
      index = history.length - 1;
    }
    for (const listener of listeners) listener(next, previous);
  };

  const step = (delta: number): void => {
    const target = index + delta;
    if (target < 0 || target >= history.length) return;
    const previous = current();
    index = target;
    for (const listener of listeners) listener(current(), previous);
  };

  const subscribe = (
    listener: (location: AppLocation, previous: AppLocation) => void,
  ): (() => void) => {
    listeners.add(listener);
    return () => listeners.delete(listener);
  };

  return {
    get: current,
    use: (select) =>
      useSyncExternalStore(
        (onChange) => subscribe(() => onChange()),
        () => select(current()),
      ),
    subscribe,
    go(patch: LocationPatch, options) {
      commit(applyPatch(current(), patch), options?.replace === true);
    },
    back: () => step(-1),
    forward: () => step(1),
    reset(location = {}) {
      history = [applyPatch(EMPTY_LOCATION, location)];
      index = 0;
    },
    entries: () => [...history],
  };
}
