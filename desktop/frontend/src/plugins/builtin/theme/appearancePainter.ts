import { definePlugin } from "@/plugins/sdk";
import { disposeOnHmr } from "@/lib/hmr";
import { useAppearanceStore } from "./adapters/appearanceStore";
import { installDocumentAppearance } from "./adapters/documentAppearance";
import { installSystemAppearance } from "./adapters/systemAppearance";
import { installAppearancePreferencePort } from "./adapters/appearancePreferenceBinding";

export const appearancePainter = definePlugin({
  name: "flame.builtin.appearance-painter",
  setup(ctx) {
    const releasePreference = installAppearancePreferencePort();
    const releaseSystem = installSystemAppearance();
    const stopPainting = installDocumentAppearance(useAppearanceStore);
    const uninstall = () => {
      stopPainting();
      releaseSystem();
      releasePreference();
    };
    disposeOnHmr(uninstall);
    ctx.cleanup(uninstall);
  },
});
