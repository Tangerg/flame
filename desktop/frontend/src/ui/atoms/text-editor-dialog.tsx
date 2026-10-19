import {
  type FormEvent,
  type KeyboardEvent,
  type ReactNode,
  useId,
  useLayoutEffect,
  useRef,
} from "react";
import { Button, FormField, isImeKey, ModalDialog, TextArea } from "@flame/ui";
import * as stylex from "@stylexjs/stylex";
import { SystemMessage } from "./system-message";

const styles = stylex.create({
  editor: { maxHeight: "40dvh" },
});

interface TextEditorDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  icon?: ReactNode;
  title: ReactNode;
  closeLabel: string;
  label: string;
  value: string;
  onChange: (value: string) => void;
  cancelLabel: string;
  saveLabel: string;
  savingLabel: string;
  busy?: boolean;
  error?: string;
  saveDisabled?: boolean;
  onSave: () => void;
  font?: "sans" | "mono";
  secondaryAction?: { label: string; onClick: () => void };
}

export function TextEditorDialog({
  open,
  onOpenChange,
  icon,
  title,
  closeLabel,
  label,
  value,
  onChange,
  cancelLabel,
  saveLabel,
  savingLabel,
  busy = false,
  error,
  saveDisabled = false,
  onSave,
  font = "sans",
  secondaryAction,
}: TextEditorDialogProps) {
  const editorRef = useRef<HTMLTextAreaElement>(null);
  const errorRef = useRef<HTMLDivElement>(null);
  const errorId = useId();
  useLayoutEffect(() => {
    if (error) errorRef.current?.scrollIntoView({ block: "nearest" });
  }, [error]);
  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (!busy && !saveDisabled) onSave();
  };
  const submitShortcut = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === "Enter" && (event.metaKey || event.ctrlKey) && !isImeKey(event.nativeEvent)) {
      event.preventDefault();
      event.currentTarget.form?.requestSubmit();
    }
  };

  return (
    <ModalDialog
      open={open}
      onOpenChange={onOpenChange}
      title={title}
      icon={icon}
      closeLabel={closeLabel}
      initialFocus={editorRef}
      onSubmit={submit}
      slot="text-editor-dialog"
      backdropSlot="text-editor-backdrop"
      auxiliaryAction={
        secondaryAction && (
          <Button type="button" variant="ghost" pending={busy} onClick={secondaryAction.onClick}>
            {secondaryAction.label}
          </Button>
        )
      }
      footer={
        <>
          <Button type="button" variant="ghost" pending={busy} onClick={() => onOpenChange(false)}>
            {cancelLabel}
          </Button>
          <Button type="submit" variant="primary" disabled={saveDisabled} pending={busy}>
            {busy ? savingLabel : saveLabel}
          </Button>
        </>
      }
    >
      <FormField label={label}>
        <TextArea
          ref={editorRef}
          rows={4}
          autosize
          styles={styles.editor}
          font={font}
          aria-label={label}
          aria-describedby={error ? errorId : undefined}
          invalid={Boolean(error)}
          value={value}
          pending={busy}
          onKeyDown={submitShortcut}
          onChange={(event) => onChange(event.target.value)}
        />
      </FormField>
      {error && (
        <SystemMessage ref={errorRef} id={errorId} variant="error">
          {error}
        </SystemMessage>
      )}
    </ModalDialog>
  );
}
