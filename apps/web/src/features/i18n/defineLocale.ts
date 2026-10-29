import { type LocaleSpec } from "@/features/i18n/public/types";
import { type AnyPlugin } from "dougong";
import { definePlugin } from "@/extensions/definePlugin";
import { LOCALE } from "@/features/i18n/public/extensionPoints";
import { activeLocale, mountLocaleBundle } from "@/lib/i18n/index";
export function defineLocale(
  spec: Omit<LocaleSpec, "activate"> & { load?: () => Promise<Record<string, string>> },
): AnyPlugin {
  return definePlugin({
    name: `flame.builtin.locale-${spec.id}`,
    setup(ctx) {
      let activation: Promise<void> | undefined;
      const activate = (): Promise<void> => {
        ctx.signal.throwIfAborted();
        if (!activation) {
          activation = ctx.spawn(async (signal) => {
            if (!spec.load) return;
            const dict = await spec.load();
            signal.throwIfAborted();
            ctx.cleanup(mountLocaleBundle(spec.id, dict));
          }).result;
          void activation.catch(() => {
            activation = undefined;
          });
        }
        return activation;
      };
      ctx.contribute(LOCALE, { id: spec.id, label: spec.label, order: spec.order, activate });
      if (spec.id === activeLocale()) void activate();
    },
  });
}
