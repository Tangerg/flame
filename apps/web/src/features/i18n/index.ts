import { definePlugin } from "@/extensions/definePlugin";
import { LOCALE } from "@/features/i18n/public/extensionPoints";

export const i18nPlugin = definePlugin({
  name: "flame.builtin.i18n",
  setup(scope) {
    scope.contribute(LOCALE, {
      id: "en",
      label: "English",
      order: -1,
      activate: async () => undefined,
    });
  },
});
