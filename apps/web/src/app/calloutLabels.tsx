import { type ReactNode } from "react";
import { CalloutLabelsProvider, type CalloutLabels } from "@flame/ui";
import { useT } from "@/lib/i18n/index";

export function ApplicationCalloutLabels({ children }: { children: ReactNode }) {
  const t = useT();
  const labels: CalloutLabels = {
    note: t("callout.note"),
    tip: t("callout.tip"),
    important: t("callout.important"),
    warning: t("callout.warning"),
    caution: t("callout.caution"),
  };
  return <CalloutLabelsProvider labels={labels}>{children}</CalloutLabelsProvider>;
}
