import type { LocaleSpec } from "@/plugins/sdk/types";
import type { AnyPlugin } from "dougong";
import { definePlugin } from "@/plugins/sdk";
import { LOCALE } from "@/plugins/sdk/kernelPoints";
import { activeLocale, addLocaleBundle } from "@/lib/i18n";

export function defineLocale(spec: LocaleSpec): AnyPlugin {
  return definePlugin({
    name: `flame.builtin.locale-${spec.id}`,
    setup(ctx) {
      ctx.contribute(LOCALE, spec);
      if (spec.load && spec.id === activeLocale()) {
        void spec.load().then((dict) => {
          addLocaleBundle(spec.id, dict);
        });
      }
    },
  });
}
