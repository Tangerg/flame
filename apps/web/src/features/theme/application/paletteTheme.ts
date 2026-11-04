import { type ContributionScope } from "@/extensions/definePlugin";
import { COLOR_THEME } from "@/features/theme/public/extensionPoints";
import { paletteThemeContribution, type PaletteTheme } from "@/features/theme/kit/palette";
import { type AppearancePreferencePort } from "@/features/theme/application/appearancePreference";
export function contributePaletteTheme(
  preference: AppearancePreferencePort,
  owner: Pick<ContributionScope, "add" | "lifetime">,
  theme: PaletteTheme,
): void {
  const initial = preference.read();
  const contribution = owner.add(
    COLOR_THEME,
    paletteThemeContribution(theme, initial.accent, initial.contrast, initial),
  );
  owner.lifetime.defer(
    preference.subscribe((next, previous) => {
      if (
        next.accent !== previous.accent ||
        next.contrast !== previous.contrast ||
        next.paperTexture !== previous.paperTexture ||
        next.paperTextureStrength !== previous.paperTextureStrength
      )
        contribution.update(paletteThemeContribution(theme, next.accent, next.contrast, next));
    }),
  );
}
