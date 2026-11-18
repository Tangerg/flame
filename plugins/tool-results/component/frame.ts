import type { CodeActionLabels } from "@flame/ui";
import type { Translate } from "@flame/component-sdk";

export const frameInset = "calc(var(--spacing) * 3)";

export function resultActions(t: Translate, copy: string): CodeActionLabels {
  return {
    copy,
    wrap: t("action.wrap"),
    unwrap: t("action.unwrap"),
  };
}
