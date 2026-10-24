import { useId, useLayoutEffect, useRef, type ReactNode } from "react";
import { Button, FormField, ModalDialog, TextField } from "@flame/ui";
import { SystemMessage } from "@/ui/atoms/system-message";
interface InputDialogProps {
  title: string;
  description: ReactNode;
  label: string;
  value: string;
  busy: boolean;
  error?: string | null;
  confirmLabel: string;
  cancelLabel: string;
  onChange(value: string): void;
  onConfirm(): void;
  onClose(): void;
  browse?: { label: string; run(): void };
}

export function InputDialog(props: InputDialogProps) {
  const field = useRef<HTMLInputElement>(null);
  const errorRef = useRef<HTMLDivElement>(null);
  const errorId = useId();
  useLayoutEffect(() => {
    if (props.error) errorRef.current?.scrollIntoView({ block: "nearest" });
  }, [props.error]);
  return (
    <ModalDialog
      open
      title={props.title}
      description={props.description}
      initialFocus={field}
      onOpenChange={(open) => {
        if (!open) props.onClose();
      }}
      onSubmit={(event) => {
        event.preventDefault();
        if (!props.busy && props.value.trim()) props.onConfirm();
      }}
      auxiliaryAction={
        props.browse && (
          <Button type="button" variant="ghost" pending={props.busy} onClick={props.browse.run}>
            {props.browse.label}
          </Button>
        )
      }
      footer={
        <>
          <Button type="button" variant="ghost" onClick={props.onClose}>
            {props.cancelLabel}
          </Button>
          <Button
            type="submit"
            variant="primary"
            disabled={!props.value.trim()}
            pending={props.busy}
          >
            {props.confirmLabel}
          </Button>
        </>
      }
    >
      <FormField label={props.label}>
        <TextField
          ref={field}
          font="mono"
          size="lg"
          aria-label={props.label}
          aria-describedby={props.error ? errorId : undefined}
          value={props.value}
          pending={props.busy}
          onChange={(event) => props.onChange(event.target.value)}
          autoComplete="off"
          spellCheck={false}
        />
      </FormField>
      {props.error && (
        <SystemMessage ref={errorRef} id={errorId} variant="error">
          {props.error}
        </SystemMessage>
      )}
    </ModalDialog>
  );
}
