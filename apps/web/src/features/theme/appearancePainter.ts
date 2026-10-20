import { installBrowserFontAvailability } from "@/features/theme/adapters/browserFontAvailability";
import { definePlugin } from "@/extensions/definePlugin";
import { useAppearanceStore } from "@/features/theme/adapters/appearanceStore";
import { installDocumentAppearance } from "@/features/theme/adapters/documentAppearance";
import { installSystemAppearance } from "@/features/theme/adapters/systemAppearance";
import { installAppearancePreferencePort } from "@/features/theme/adapters/appearancePreferenceBinding";
export const appearancePainter = definePlugin({
  name: "flame.builtin.appearance-painter",
  setup(ctx) {
    ctx.cleanup(installBrowserFontAvailability());
    ctx.cleanup(installAppearancePreferencePort());
    ctx.cleanup(installSystemAppearance());
    ctx.cleanup(installDocumentAppearance(useAppearanceStore));
  },
});
