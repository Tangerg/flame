import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import { Button } from "@flame/ui";
import { formDialog, MODAL_SCRIM, modalPanel } from "@flame/ui/floating-surface";
import { space, type } from "@flame/ui/tokens.stylex";
import { DialogPrimitive } from "@/ui/primitives";

const styles = stylex.create({
  panel: {
    width: "min(640px, calc(100vw - 32px))",
    maxHeight: "calc(100vh - 32px)",
    overflowY: "auto",
  },
  content: { marginBlock: space.s3 },
});

export function ContentDialog({
  title,
  closeLabel,
  close,
  children,
}: {
  title: ReactNode;
  closeLabel: string;
  close(): void;
  children: ReactNode;
}) {
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
          <DialogPrimitive.Title {...stylex.props(type.displaySm)}>{title}</DialogPrimitive.Title>
          <div {...stylex.props(styles.content)}>{children}</div>
          <Button variant="ghost" onClick={close}>
            {closeLabel}
          </Button>
        </DialogPrimitive.Popup>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  );
}
