import type { LocaleSpec } from "@/plugins/sdk/types";
import type { AnyPlugin } from "dougong";
import { definePlugin } from "@/plugins/sdk";
import { LOCALE } from "@/plugins/sdk/kernelPoints";
import { activeLocale, mountLocaleBundle } from "@/lib/i18n";

export function defineLocale(
  spec: Omit<LocaleSpec, "activate"> & { load?: () => Promise<Record<string, string>> },
): AnyPlugin {
  return definePlugin({
    name: `flame.builtin.locale-${spec.id}`,
    setup(ctx) {
      let activation: Promise<void> | undefined;
      const activate = (): Promise<void> => {
        ctx.signal.throwIfAborted();
        return (activation ??= ctx.spawn(async (signal) => {
          if (!spec.load) return;
          const dict = await spec.load();
          signal.throwIfAborted();
          ctx.cleanup(mountLocaleBundle(spec.id, dict));
        }).result);
      };
      ctx.contribute(LOCALE, { id: spec.id, label: spec.label, order: spec.order, activate });
      if (spec.id === activeLocale()) void activate();
    },
  });
}
