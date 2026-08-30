// The four scalars below are the whole answer to "what am I looking at", and they live in
// router search params so browser history contains the complete location.
//
// THE OWNERSHIP RULE, because getting it wrong is how a URL becomes a mirror: the location
// owns where you ARE, stores own what you KEPT. A transition may READ memory to seed the
// location it navigates to; nothing writes memory back into the location behind the user's
// back, and nothing keeps a second copy of these four.
//
// A port rather than a direct router import, so tests and visual fixtures can install an
// in-memory navigator instead of standing up a router they have no other use for.

import { createSingletonPort } from "./ports/singletonPort";

export interface AppLocation {
  /** Active session id; "" when none is selected. */
  session: string;
  /** A workspace view promoted to the whole content card; null is the chat. */
  view: string | null;
  /** The dock destination beside the chat; null means the dock is collapsed. */
  dock: string | null;
  /** The open settings pane; null when settings are closed. */
  settings: string | null;
}

export const EMPTY_LOCATION: AppLocation = {
  session: "",
  view: null,
  dock: null,
  settings: null,
};

export type LocationPatch = Partial<AppLocation>;

export interface Navigator {
  get(): AppLocation;
  /**
   * Reactive read. Select ONE field: the selected value is compared by identity,
   * so returning the whole location re-renders on every navigation.
   */
  use<T>(select: (location: AppLocation) => T): T;
  subscribe(listener: (location: AppLocation, previous: AppLocation) => void): () => void;
  /**
   * Omitted fields keep their value; `null` clears one. `replace` is for corrections that
   * were never a place the user went, such as seeding the last session on a cold start.
   *
   * Do NOT wrap this in `startTransition` expecting the render to be deferred: React
   * de-opts a transition to a SYNCHRONOUS render when the update arrives through
   * `useSyncExternalStore`, which is how both the location and the transcript reach a
   * component. Nothing is deferred and the extra render is spent for nothing.
   */
  go(patch: LocationPatch, options?: { replace?: boolean }): void;
  back(): void;
  forward(): void;
}

const port = createSingletonPort<Navigator>("Navigator port is not configured");

export const configureNavigator = port.configure;
export const navigator = port.get;

export function applyPatch(location: AppLocation, patch: LocationPatch): AppLocation {
  return { ...location, ...patch };
}

export function sameLocation(a: AppLocation, b: AppLocation): boolean {
  return (
    a.session === b.session && a.view === b.view && a.dock === b.dock && a.settings === b.settings
  );
}
