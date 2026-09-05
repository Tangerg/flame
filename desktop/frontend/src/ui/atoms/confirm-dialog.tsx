import type { ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { Button } from "./button";
import { DialogPrimitive } from "@/ui/primitives";
import { FLOATING_MOTION, MODAL_SCRIM } from "./floating-surface";

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
        <DialogPrimitive.Backdrop data-slot="confirm-dialog-backdrop" className={MODAL_SCRIM} />
        <DialogPrimitive.Popup
          data-slot="confirm-dialog"
          // A destructive confirmation IS an alert: it interrupts to demand an answer before
          // something goes away for good, which is the one thing `alertdialog` is for. Plain
          // `dialog` announces it as another window and loses the urgency the copy is
          // carrying. A confirmation that only asks — none exists yet — is not an alert.
          role={destructive ? "alertdialog" : undefined}
          className={cn(
            "fixed inset-0 z-[var(--layer-modal)] m-auto h-fit w-[min(400px,calc(100vw-32px))]",
            "rounded-[var(--floating-panel-radius)] bg-canvas p-4 shadow-[var(--shadow-modal)] outline-none",
            FLOATING_MOTION,
          )}
        >
          <DialogPrimitive.Title className="text-display-sm font-semibold text-fg">
            {title}
          </DialogPrimitive.Title>
          <DialogPrimitive.Description className="mt-1.5 text-ui-md leading-relaxed text-fg-muted">
            {body}
          </DialogPrimitive.Description>
          <div className="mt-4 flex items-center justify-end gap-2">
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
