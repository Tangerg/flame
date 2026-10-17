import { createElement, Suspense } from "react";
import type { CapturedSession } from "@flame/plugin-sdk";
import type { SessionActionSpec } from "@/plugins/sdk/types";
import { PluginBoundary } from "@/plugins/host/PluginBoundary";
import { ModalDialog } from "@flame/ui";
import { useT } from "@/lib/i18n";

export function SessionActionDialog({
  action,
  session,
  close,
}: {
  action: SessionActionSpec;
  session: CapturedSession;
  close(): void;
}) {
  const t = useT();
  return (
    <ModalDialog
      open
      title={action.title}
      closeLabel={t("common.close")}
      size="lg"
      onOpenChange={(open) => {
        if (!open) close();
      }}
    >
      <PluginBoundary plugin={action.id} label={action.title}>
        <Suspense fallback={<p role="status">{t("packages.module.loading")}</p>}>
          {createElement(action.component, { session })}
        </Suspense>
      </PluginBoundary>
    </ModalDialog>
  );
}
