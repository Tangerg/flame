import * as token from "@flame/ui/tokens.stylex";
import type { CodeActionLabels } from "@flame/ui";
import type { Translate } from "@flame/component-sdk";

export const frameInset = token.space.s3;

export function resultActions(t: Translate, copy: string): CodeActionLabels {
  return {
    copy,
    wrap: t("action.wrap"),
    unwrap: t("action.unwrap"),
  };
}
