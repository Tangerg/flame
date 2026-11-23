import { type Translate } from "@/lib/i18n/index";

export function sessionDisplayTitle(t: Translate, title: string | undefined): string {
  return title?.trim() || t("session.untitled");
}
