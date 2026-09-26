import * as stylex from "@stylexjs/stylex";
import { useRef, type ReactNode } from "react";
import { DialogPrimitive } from "@/ui/primitives";
import { color, space, type, weight } from "@/styles/tokens.stylex";
import { Button } from "./button";
import { TextField } from "./text-field";
import { formDialog, MODAL_SCRIM, modalPanel } from "./floating-surface";

const styles = stylex.create({
  panel: { width: "min(480px, calc(100vw - 32px))" },
  title: { fontWeight: weight.semibold, color: color.fg },
  description: { color: color.fgMuted, marginBlock: space.s3, overflowWrap: "anywhere" },
  leading: { marginRight: "auto" },
});

interface InputDialogProps {
  title: string;
  description: ReactNode;
  label: string;
  value: string;
  busy: boolean;
  confirmLabel: string;
  cancelLabel: string;
  onChange(value: string): void;
  onConfirm(): void;
  onClose(): void;
  browse?: { label: string; run(): void };
}

export function InputDialog(props: InputDialogProps) {
  const field = useRef<HTMLInputElement>(null);
  return (
    <DialogPrimitive.Root
      open
      onOpenChange={(open) => {
        if (!open) props.onClose();
      }}
    >
      <DialogPrimitive.Portal>
        <DialogPrimitive.Backdrop className={stylex.props(MODAL_SCRIM).className} />
        <DialogPrimitive.Popup
          initialFocus={field}
          {...stylex.props(modalPanel(), formDialog.plane, formDialog.inset, styles.panel)}
        >
          <form
            onSubmit={(event) => {
              event.preventDefault();
              if (!props.busy && props.value.trim()) props.onConfirm();
            }}
          >
            <DialogPrimitive.Title {...stylex.props(type.displaySm, styles.title)}>
              {props.title}
            </DialogPrimitive.Title>
            <DialogPrimitive.Description {...stylex.props(type.uiMd, styles.description)}>
              {props.description}
            </DialogPrimitive.Description>
            <TextField
              ref={field}
              font="mono"
              aria-label={props.label}
              value={props.value}
              pending={props.busy}
              onChange={(event) => props.onChange(event.target.value)}
              autoComplete="off"
              spellCheck={false}
            />
            <div {...stylex.props(formDialog.actions)}>
              {props.browse && (
                <Button
                  type="button"
                  variant="ghost"
                  pending={props.busy}
                  onClick={props.browse.run}
                  styles={styles.leading}
                >
                  {props.browse.label}
                </Button>
              )}
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
            </div>
          </form>
        </DialogPrimitive.Popup>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  );
}
