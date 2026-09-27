import { definePlugin } from "@/plugins/sdk";
import { useAppearanceStore } from "./adapters/appearanceStore";
import { installDocumentAppearance } from "./adapters/documentAppearance";
import { installSystemAppearance } from "./adapters/systemAppearance";
import { installAppearancePreferencePort } from "./adapters/appearancePreferenceBinding";

export const appearancePainter = definePlugin({
  name: "flame.builtin.appearance-painter",
  setup(ctx) {
    ctx.cleanup(installAppearancePreferencePort());
    ctx.cleanup(installSystemAppearance());
    ctx.cleanup(installDocumentAppearance(useAppearanceStore));
  },
});
