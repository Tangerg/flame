import { createElement, Suspense, type RefObject } from "react";
import { type CapturedSession } from "@flame/component-sdk";
import { type SessionActionSpec } from "@/features/navigation/public/sessionActions";
import { PluginBoundary } from "@/extensions/host/PluginBoundary";
import { ModalDialog } from "@flame/ui";
import { presentationTitle } from "@/lib/i18n/presentationTitle";
import { useLocale, useT } from "@/lib/i18n/index";
import { sessionDisplayTitle } from "@/features/agent/public/session";
export function SessionActionDialog({
  action,
  session,
  close,
  finalFocus,
}: {
  action: SessionActionSpec;
  session: CapturedSession;
  close(): void;
  finalFocus: RefObject<HTMLElement | null>;
}) {
  const t = useT();
  const title = presentationTitle(action.title, action.localizedTitles, useLocale());
  return (
    <ModalDialog
      open
      title={title}
      description={sessionDisplayTitle(t, session.title)}
      closeLabel={t("common.close")}
      size="lg"
      finalFocus={finalFocus}
      onOpenChange={(open) => {
        if (!open) close();
      }}
    >
      <PluginBoundary plugin={action.id} label={title}>
        <Suspense fallback={<p role="status">{t("packages.module.loading")}</p>}>
          {createElement(action.component, { session })}
        </Suspense>
      </PluginBoundary>
    </ModalDialog>
  );
}
