// The appearance preference, for the pane that edits it and the footer that names the
// active theme. Every DERIVED value — tokens, scheme, motion — is published downward by the
// painter instead, so nothing outside this context recomputes what the document shows.
export { editAppearance, useAppearance } from "../application/appearancePreference";
export {
  ACCENT_TINTS,
  UI_DENSITY_MODES,
  type AccentTint,
  type ColorThemeId,
  type UiDensity,
} from "../kit/appearance";
