import type { LocaleSpec } from "@/plugins/sdk/types";
import { setLocale } from "@/lib/i18n";

export async function selectLocale(spec: LocaleSpec): Promise<void> {
  const activation = spec.activate();
  setLocale(spec.id);
  await activation;
}
