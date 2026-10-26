import { type AppearanceEdit, type AppearancePreference } from "@/features/theme/kit/appearance";
export interface AppearancePreferencePort {
  read(): AppearancePreference;
  edit(): AppearanceEdit;
  subscribe(
    listener: (next: AppearancePreference, previous: AppearancePreference) => void,
  ): () => void;
}
