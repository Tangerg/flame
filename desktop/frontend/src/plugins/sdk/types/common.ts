// Cross-cutting primitives shared by every other types file.

/**
 * A reversible application-level handle. Dougong owns plugin setup resources;
 * this narrower shape remains for UI registrations outside its core contracts.
 */
export interface Disposable {
  dispose: () => void;
}

/**
 * Fires once, after every built-in plugin has loaded; registering after that point fires
 * immediately, so "have I missed it" is never a concern. Use it for setup that must read the
 * full registry — registering at setup time is order-dependent, deferring here is not.
 */
export type ReadyHandler = () => void;
