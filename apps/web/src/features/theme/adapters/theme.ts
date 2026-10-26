import { createAppearanceProjection } from "@/lib/appearance";
import { createAppearanceStore } from "@/features/theme/adapters/appearanceStore";
import { createSystemAppearance } from "@/features/theme/adapters/systemAppearance";
import { createBrowserFontAvailability } from "@/features/theme/adapters/browserFontAvailability";
import { installDocumentAppearance } from "@/features/theme/adapters/documentAppearance";
import { type ThemeAccess } from "@/features/theme/application/themeAccess";

export function createTheme(document: Document, window: Window) {
  const store = createAppearanceStore(() => window.localStorage);
  const system = createSystemAppearance(window.matchMedia("(prefers-color-scheme: dark)"));
  const appearance = createAppearanceProjection();
  const access: ThemeAccess = {
    preference: {
      read: store.getState,
      edit: store.getState,
      subscribe: store.subscribe,
    },
    system,
    fonts: createBrowserFontAvailability(document),
    appearance,
  };
  let stop: (() => void) | undefined;
  let disposed = false;
  return {
    access,
    store,
    appearance,
    start() {
      if (disposed) throw new Error("Theme is disposed");
      stop?.();
      const release = installDocumentAppearance(document, window, store, system, appearance);
      stop = release;
      return () => {
        release();
        if (stop === release) stop = undefined;
      };
    },
    dispose() {
      if (disposed) return;
      disposed = true;
      stop?.();
      stop = undefined;
    },
  };
}
export type Theme = ReturnType<typeof createTheme>;
