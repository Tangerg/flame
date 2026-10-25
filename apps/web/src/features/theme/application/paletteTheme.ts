import { type ContributionLifetime } from "@/extensions/definePlugin";
import { COLOR_THEME } from "@/features/theme/public/extensionPoints";
import { paletteThemeContribution, type PaletteTheme } from "@/features/theme/kit/palette";
import { appearancePreferencePort } from "@/features/theme/application/ports/appearancePreference";
export function contributePaletteTheme(
  owner: Pick<ContributionLifetime, "contribute" | "cleanup">,
  theme: PaletteTheme,
): void {
  const preference = appearancePreferencePort();
  const initial = preference.read();
  const contribution = owner.contribute(
    COLOR_THEME,
    paletteThemeContribution(theme, initial.accent, initial.contrast, initial),
  );
  owner.cleanup(
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
