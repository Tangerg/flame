import { createSingletonPort } from "@/lib/ports/singletonPort";
import type { AppearanceEdit, AppearancePreference } from "../../kit/appearance";

interface AppearancePreferencePort {
  use<T>(select: (preference: AppearancePreference) => T): T;
  read(): AppearancePreference;
  edit(): AppearanceEdit;
  subscribe(
    listener: (next: AppearancePreference, previous: AppearancePreference) => void,
  ): () => void;
}

const port = createSingletonPort<AppearancePreferencePort>(
  "Appearance preference port is not configured",
);

export const configureAppearancePreferencePort = port.configure;
export const appearancePreferencePort = port.get;
