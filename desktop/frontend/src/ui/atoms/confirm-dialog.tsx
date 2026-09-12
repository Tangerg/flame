import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import { color, leading, radius, space, surface, type, weight } from "@/styles/tokens.stylex";
import { Button } from "./button";
import { DialogPrimitive } from "@/ui/primitives";
import { MODAL_SCRIM, modalPanel } from "./floating-surface";

const styles = stylex.create({
  panel: {
    width: "min(400px, calc(100vw - 32px))",
    borderRadius: radius.floatingPanel,
    backgroundColor: surface.canvas,
    padding: space.s4,
  },
  title: { fontWeight: weight.semibold, color: color.fg },
  body: { marginTop: space.s1_5, lineHeight: leading.relaxed, color: color.fgMuted },
  actions: {
    marginTop: space.s4,
    display: "flex",
    alignItems: "center",
    justifyContent: "flex-end",
    gap: space.s2,
  },
});

interface ConfirmDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: ReactNode;
  body: ReactNode;
  confirmLabel: string;
  cancelLabel: string;
  destructive?: boolean;
  onConfirm: () => void;
}

export function ConfirmDialog({
  open,
  onOpenChange,
  title,
  body,
  confirmLabel,
  cancelLabel,
  destructive,
  onConfirm,
}: ConfirmDialogProps) {
  return (
    <DialogPrimitive.Root open={open} onOpenChange={onOpenChange}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Backdrop
          data-slot="confirm-dialog-backdrop"
          className={stylex.props(MODAL_SCRIM).className}
        />
        <DialogPrimitive.Popup
          data-slot="confirm-dialog"
          // A destructive confirmation IS an alert: it interrupts to demand an answer before
          // something goes away for good, which is the one thing `alertdialog` is for. Plain
          // `dialog` announces it as another window and loses the urgency the copy is
          // carrying. A confirmation that only asks — none exists yet — is not an alert.
          role={destructive ? "alertdialog" : undefined}
          {...stylex.props(modalPanel(), styles.panel)}
        >
          <DialogPrimitive.Title {...stylex.props(type.displaySm, styles.title)}>
            {title}
          </DialogPrimitive.Title>
          <DialogPrimitive.Description {...stylex.props(type.uiMd, styles.body)}>
            {body}
          </DialogPrimitive.Description>
          <div {...stylex.props(styles.actions)}>
            <DialogPrimitive.Close render={<Button variant="ghost">{cancelLabel}</Button>} />
            <Button
              variant={destructive ? "tonal" : "primary"}
              tone={destructive ? "negative" : undefined}
              onClick={() => {
                onOpenChange(false);
                onConfirm();
              }}
            >
              {confirmLabel}
            </Button>
          </div>
        </DialogPrimitive.Popup>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  );
}
