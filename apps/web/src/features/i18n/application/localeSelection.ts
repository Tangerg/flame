import { type LocaleSpec } from "@/features/i18n/public/types";
import { setLocale } from "@/lib/i18n/index";
export async function selectLocale(spec: LocaleSpec): Promise<void> {
  setLocale(spec.id);
  await spec.activate();
}
