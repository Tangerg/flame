import { createSingletonPort } from "./ports/singletonPort";

export interface AppLocation {
  session: string;
  view: string | null;
  dock: string | null;
  settings: string | null;
  subagent: string | null;
}

export const EMPTY_LOCATION: AppLocation = {
  session: "",
  view: null,
  dock: null,
  settings: null,
  subagent: null,
};

export type LocationPatch = Partial<AppLocation>;

export interface Navigator {
  get(): AppLocation;
  use<T>(select: (location: AppLocation) => T): T;
  subscribe(listener: (location: AppLocation, previous: AppLocation) => void): () => void;
  go(patch: LocationPatch, options?: { replace?: boolean }): void;
  back(): void;
  forward(): void;
}

const port = createSingletonPort<Navigator>("Navigator port is not configured");

export const configureNavigator = port.configure;
export const navigator = port.get;

export function applyPatch(location: AppLocation, patch: LocationPatch): AppLocation {
  return {
    ...location,
    ...(patch.session !== undefined && patch.session !== location.session
      ? { subagent: null }
      : {}),
    ...patch,
  };
}

export function sameLocation(a: AppLocation, b: AppLocation): boolean {
  return (
    a.session === b.session &&
    a.view === b.view &&
    a.dock === b.dock &&
    a.settings === b.settings &&
    a.subagent === b.subagent
  );
}
