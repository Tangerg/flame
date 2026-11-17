import { type CodeActionLabels } from "@flame/ui";
import { useT } from "@/lib/i18n/index";
export function useCodeActionLabels(): CodeActionLabels {
  const t = useT();
  return {
    copy: t("message.code.copy"),
    wrap: t("message.code.wrap.enable"),
    unwrap: t("message.code.wrap.disable"),
  };
}
