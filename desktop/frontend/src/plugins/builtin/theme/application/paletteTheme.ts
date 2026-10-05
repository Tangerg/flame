import type { ContributionLifetime } from "@/plugins/sdk/definePlugin";
import { COLOR_THEME } from "@/plugins/sdk/kernelPoints";
import { paletteThemeContribution, type PaletteTheme } from "../kit/palette";
import { appearancePreferencePort } from "./ports/appearancePreference";

export function contributePaletteTheme(
  owner: Pick<ContributionLifetime, "contribute" | "cleanup">,
  theme: PaletteTheme,
): void {
  const preference = appearancePreferencePort();
  const initial = preference.read();
  const contribution = owner.contribute(
    COLOR_THEME,
    paletteThemeContribution(theme, initial.accent, initial.contrast),
  );
  owner.cleanup(
    preference.subscribe((next, previous) => {
      if (next.accent !== previous.accent || next.contrast !== previous.contrast)
        contribution.update(paletteThemeContribution(theme, next.accent, next.contrast));
    }),
  );
}
