export interface AppLocation {
  session: string;
  view: string | null;
  dock: string | null;
  settings: string | null;
  subagent: string | null;
  // Addresses an object inside the shown view, in that view's own terms.
  target: string | null;
}

export const EMPTY_LOCATION: AppLocation = {
  session: "",
  view: null,
  dock: null,
  settings: null,
  subagent: null,
  target: null,
};

export type LocationPatch = Partial<AppLocation>;

export interface Navigator {
  get(): AppLocation;
  use<T>(select: (location: AppLocation) => T): T;
  subscribe(listener: (location: AppLocation, previous: AppLocation) => void): () => void;
  go(patch: LocationPatch, options?: { replace?: boolean }): void;
  // The location the patch would produce, as an absolute URL.
  href(patch: LocationPatch): string;
  back(): void;
  forward(): void;
}

// Showing another view drops the target, which addressed the previous one.
export function applyPatch(location: AppLocation, patch: LocationPatch): AppLocation {
  const shown = (next: Pick<AppLocation, "view" | "dock">) => next.view ?? next.dock;
  return {
    ...location,
    ...(patch.session !== undefined && patch.session !== location.session
      ? { subagent: null }
      : {}),
    ...(shown({ ...location, ...patch }) !== shown(location) ? { target: null } : {}),
    ...patch,
  };
}

export function sameLocation(a: AppLocation, b: AppLocation): boolean {
  return (
    a.session === b.session &&
    a.view === b.view &&
    a.dock === b.dock &&
    a.settings === b.settings &&
    a.subagent === b.subagent &&
    a.target === b.target
  );
}
