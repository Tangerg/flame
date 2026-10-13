import { createElement, Suspense } from "react";
import * as stylex from "@stylexjs/stylex";
import { Button } from "@flame/ui";
import { formDialog, MODAL_SCRIM, modalPanel } from "@flame/ui/floating-surface";
import { space, type } from "@flame/ui/tokens.stylex";
import type { CapturedSession } from "@flame/plugin-sdk";
import type { SessionActionSpec } from "@/plugins/sdk/types";
import { PluginBoundary } from "@/plugins/host/PluginBoundary";
import { DialogPrimitive } from "@/ui/primitives";
import { useT } from "@/lib/i18n";

const styles = stylex.create({
  panel: {
    width: "min(640px, calc(100vw - 32px))",
    maxHeight: "calc(100vh - 32px)",
    overflowY: "auto",
  },
  content: { marginBlock: space.s3 },
});

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
    <DialogPrimitive.Root
      open
      onOpenChange={(open) => {
        if (!open) close();
      }}
    >
      <DialogPrimitive.Portal>
        <DialogPrimitive.Backdrop className={stylex.props(MODAL_SCRIM).className} />
        <DialogPrimitive.Popup
          {...stylex.props(modalPanel(), formDialog.plane, formDialog.inset, styles.panel)}
        >
          <DialogPrimitive.Title {...stylex.props(type.displaySm)}>
            {action.title}
          </DialogPrimitive.Title>
          <div {...stylex.props(styles.content)}>
            <PluginBoundary plugin={action.id} label={action.title}>
              <Suspense fallback={<p role="status">{t("packages.module.loading")}</p>}>
                {createElement(action.component, { session })}
              </Suspense>
            </PluginBoundary>
          </div>
          <Button variant="ghost" onClick={close}>
            {t("common.close")}
          </Button>
        </DialogPrimitive.Popup>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  );
}
