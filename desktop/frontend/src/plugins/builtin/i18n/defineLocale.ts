// Convenience wrapper for the built-in locale plugins: each is just
// `definePlugin` → `contribute(LOCALE, spec)` with a name derived from the
// language tag. Lives in this plugin package — NOT the core SDK — mirroring
// `defineThemePlugin` / `defineWorkspaceView`: the kernel exposes only the
// generic `contribute` write path; per-domain ergonomics belong to the domain.

import type { LocaleSpec } from "@/plugins/sdk/types";
import type { AnyPlugin } from "dougong";
import { definePlugin } from "@/plugins/sdk";
import { LOCALE } from "@/plugins/sdk/kernelPoints";
import { activeLocale, addLocaleBundle } from "@/lib/i18n";

/**
 * Registers the picker entry ONLY — `load` fetches the dictionary on first selection, since
 * statically importing every language put eight dictionaries in the entry payload, seven of
 * which the reader never sees. English omits `load`: lib/i18n bootstraps it so first paint
 * always has strings.
 */
export function defineLocale(spec: LocaleSpec): AnyPlugin {
  return definePlugin({
    name: `flame.builtin.locale-${spec.id}`,
    setup(ctx) {
      ctx.contribute(LOCALE, spec);
      // Cold start with a persisted non-English locale: this plugin is the only
      // thing that knows how to fetch its own dictionary, so it does — during
      // setup, which runs before first paint.
      if (spec.load && spec.id === activeLocale()) {
        void spec.load().then((dict) => {
          addLocaleBundle(spec.id, dict);
        });
      }
    },
  });
}
