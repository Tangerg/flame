import { useRef, type ComponentProps, type ReactNode } from "react";
import { Button, ModalDialog } from "@flame/ui";
interface ConfirmDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onOpenChangeComplete?: (open: boolean) => void;
  title: ReactNode;
  body: ReactNode;
  confirmLabel: string;
  cancelLabel: string;
  destructive?: boolean;
  finalFocus?: ComponentProps<typeof ModalDialog>["finalFocus"];
  onConfirm: () => void;
}

export function ConfirmDialog({
  open,
  onOpenChange,
  onOpenChangeComplete,
  title,
  body,
  confirmLabel,
  cancelLabel,
  destructive,
  finalFocus,
  onConfirm,
}: ConfirmDialogProps) {
  const cancel = useRef<HTMLButtonElement>(null);
  return (
    <ModalDialog
      open={open}
      onOpenChange={onOpenChange}
      onOpenChangeComplete={onOpenChangeComplete}
      title={title}
      description={body}
      size="sm"
      role={destructive ? "alertdialog" : "dialog"}
      backdropSlot="confirm-dialog-backdrop"
      initialFocus={cancel}
      finalFocus={finalFocus}
      footer={
        <>
          <Button ref={cancel} variant="ghost" onClick={() => onOpenChange(false)}>
            {cancelLabel}
          </Button>
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
        </>
      }
    />
  );
}
