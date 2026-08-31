import { createSingletonPort } from "@/lib/ports/singletonPort";
import type { AppearanceEdit, AppearancePreference } from "../../kit/appearance";

/**
 * The stored appearance choice, as this context and the pane that edits it need it.
 * Reached through a port rather than the store directly: deciding which theme comes next
 * is this context's rule, and a rule that imports a store cannot be exercised without one.
 *
 * A selector rather than a hook per field: thirteen preferences would otherwise mean
 * twenty-six port methods that all say the same thing. What the port is actually for is
 * keeping `setState`, `subscribe` and the persist middleware off every consumer.
 */
export interface AppearancePreferencePort {
  use<T>(select: (preference: AppearancePreference) => T): T;
  /** For rules that run outside render. */
  read(): AppearancePreference;
  edit(): AppearanceEdit;
}

const port = createSingletonPort<AppearancePreferencePort>(
  "Appearance preference port is not configured",
);

export const configureAppearancePreferencePort = port.configure;
export const appearancePreferencePort = port.get;
