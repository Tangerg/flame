import { type LocaleSpec } from "@/features/i18n/public/types";
import { defineExtensionPoint } from "@/extensions/contracts";
export const LOCALE = defineExtensionPoint<LocaleSpec>({
  id: "flame.locale",
  keying: "single",
});
