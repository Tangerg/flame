import type { LocaleSpec } from "@/plugins/sdk/types";
import { addLocaleBundle, setLocale } from "@/lib/i18n";

export async function selectLocale(spec: LocaleSpec): Promise<void> {
  if (spec.load) addLocaleBundle(spec.id, await spec.load());
  setLocale(spec.id);
}
