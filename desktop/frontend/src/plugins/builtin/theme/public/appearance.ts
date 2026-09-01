// The appearance preference, for the pane that edits it and the footer that names the
// active theme. Derived values — tokens, scheme, motion — are published downward by the
// painter instead, so nothing outside this context recomputes what the document shows.

import type { AppearanceEdit, AppearancePreference } from "../kit/appearance";
import { appearancePreferencePort } from "../application/ports/appearancePreference";

export function useAppearance<T>(select: (preference: AppearancePreference) => T): T {
  return appearancePreferencePort().use(select);
}

export function editAppearance(): AppearanceEdit {
  return appearancePreferencePort().edit();
}

export { ACCENT_TINTS, UI_DENSITY_MODES, type AccentTint, type UiDensity } from "../kit/appearance";
