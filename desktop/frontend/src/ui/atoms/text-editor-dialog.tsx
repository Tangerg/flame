import * as stylex from "@stylexjs/stylex";
import { type FormEvent, type KeyboardEvent, type ReactNode, useRef } from "react";
import { Button } from "./button";
import { MODAL_SCRIM, modalPanel } from "./floating-surface";
import { color, radius, space, surface, type, weight } from "@/styles/tokens.stylex";
import { IconButton } from "./icon-button";
import { TextArea } from "./text-field";
import { DialogPrimitive } from "@/ui/primitives";

const styles = stylex.create({
  // The only modal on `--radius-composer` rather than the floating-panel corner. It holds a
  // composer, so it may be deliberate — or drift. Reported, not changed.
  panel: {
    width: "min(420px, calc(100vw - 32px))",
    overflow: "hidden",
    borderRadius: radius.composer,
    backgroundColor: surface.card,
  },
  form: { position: "relative", display: "flex", flexDirection: "column", padding: space.s5 },
  head: {
    display: "flex",
    width: "100%",
    flexDirection: "column",
    alignItems: "flex-start",
    gap: space.s3,
  },
  icon: {
    display: "flex",
    height: space.s9,
    width: space.s9,
    flexShrink: 0,
    alignItems: "center",
    justifyContent: "center",
    borderRadius: radius.xl,
    backgroundColor: surface.surface2,
    padding: space.s2,
  },
  title: { fontWeight: weight.semibold, color: color.fg },
  close: { position: "absolute", top: space.s4, right: space.s4 },
  field: { display: "flex", width: "100%", flexDirection: "column", paddingTop: space.s3 },
  actions: {
    display: "flex",
    width: "100%",
    alignItems: "center",
    justifyContent: "flex-end",
    gap: space.s3,
    paddingTop: space.s3,
  },
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
  saveDisabled?: boolean;
  onSave: () => void;
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
  saveDisabled = false,
  onSave,
}: TextEditorDialogProps) {
  const editorRef = useRef<HTMLTextAreaElement>(null);
  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (!busy && !saveDisabled) onSave();
  };
  const submitShortcut = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (
      event.key === "Enter" &&
      (event.metaKey || event.ctrlKey) &&
      !event.nativeEvent.isComposing
    ) {
      event.preventDefault();
      event.currentTarget.form?.requestSubmit();
    }
  };

  return (
    <DialogPrimitive.Root open={open} onOpenChange={onOpenChange}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Backdrop
          data-slot="text-editor-backdrop"
          className={stylex.props(MODAL_SCRIM).className}
        />
        <DialogPrimitive.Popup
          data-slot="text-editor-dialog"
          initialFocus={editorRef}
          {...stylex.props(modalPanel(), styles.panel)}
        >
          <form {...stylex.props(styles.form)} onSubmit={submit}>
            <div {...stylex.props(styles.head)}>
              {icon && <span {...stylex.props(styles.icon)}>{icon}</span>}
              <DialogPrimitive.Title {...stylex.props(type.displaySm, styles.title)}>
                {title}
              </DialogPrimitive.Title>
            </div>
            <DialogPrimitive.Close
              render={
                <IconButton
                  icon="x"
                  size="xs"
                  iconSize="xs"
                  quiet
                  title={closeLabel}
                  {...stylex.props(styles.close)}
                />
              }
            />
            <div {...stylex.props(styles.field)}>
              <TextArea
                ref={editorRef}
                rows={12}
                font="sans"
                aria-label={label}
                value={value}
                pending={busy}
                onKeyDown={submitShortcut}
                onChange={(event) => onChange(event.target.value)}
              />
            </div>
            <div {...stylex.props(styles.actions)}>
              <Button
                type="button"
                variant="soft"
                pending={busy}
                onClick={() => onOpenChange(false)}
              >
                {cancelLabel}
              </Button>
              <Button type="submit" variant="primary" disabled={saveDisabled} pending={busy}>
                {busy ? savingLabel : saveLabel}
              </Button>
            </div>
          </form>
        </DialogPrimitive.Popup>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  );
}
