import { type AnyPlugin } from "dougong";
import { definePlugin } from "@/extensions/definePlugin";
import { COLOR_THEME } from "@/features/theme/public/extensionPoints";
import { type AppearancePreference } from "@/features/theme/kit/appearance";
import { type AppearancePreferencePort } from "@/features/theme/application/appearancePreference";
import { paletteThemeContribution, type PaletteTheme } from "@/features/theme/kit/palette";
export function paletteThemePlugin(
  preference: AppearancePreferencePort,
  name: string,
  theme: (preference: AppearancePreference) => PaletteTheme,
): AnyPlugin {
  return definePlugin({
    name,
    setup(ctx) {
      const derive = (preference: AppearancePreference) =>
        paletteThemeContribution(
          theme(preference),
          preference.accent,
          preference.contrast,
          preference,
        );
      const contribution = ctx.contribute(COLOR_THEME, derive(preference.read()));
      ctx.cleanup(
        preference.subscribe((next, previous) => {
          if (
            next.customTheme !== previous.customTheme ||
            next.accent !== previous.accent ||
            next.contrast !== previous.contrast ||
            next.paperTexture !== previous.paperTexture ||
            next.paperTextureStrength !== previous.paperTextureStrength
          )
            contribution.update(derive(next));
        }),
      );
    },
  });
}
