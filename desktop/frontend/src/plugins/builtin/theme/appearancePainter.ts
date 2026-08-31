// Installs during SETUP, not at module eval: the painter resolves palette and style
// contributions, so evaluating earlier runs before any theme has registered and applies
// nothing. First paint is safe either way — index.html sets the scheme class inline from
// localStorage before any module loads.

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
    // Before the painter: its first paint resolves the scheme, which asks this.
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
