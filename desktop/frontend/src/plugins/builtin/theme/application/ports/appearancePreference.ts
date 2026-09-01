import { createSingletonPort } from "@/lib/ports/singletonPort";
import type { AppearanceEdit, AppearancePreference } from "../../kit/appearance";

/** A selector rather than a hook per field: thirteen preferences would otherwise mean
 *  twenty-six methods saying the same thing. The port exists to keep `setState`,
 *  `subscribe` and the persist middleware off every consumer. */
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
