import * as stylex from "@stylexjs/stylex";
import { useRef, type KeyboardEventHandler, type ReactElement, type ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { radius, space, surface } from "@/styles/tokens.stylex";
import { DialogPrimitive } from "@/ui/primitives";
import { MODAL_SCRIM, modalPanel } from "./floating-surface";

type LightboxKind = "figure" | "document" | "media";

const styles = stylex.create({
  figure: {
    width: "fit-content",
    maxHeight: "90vh",
    maxWidth: "min(1400px, 95vw)",
    overflow: "auto",
    borderRadius: radius.floatingPanel,
    backgroundColor: surface.card,
    padding: space.s6,
  },
  document: {
    width: "fit-content",
    minWidth: "min(408px, 80vw)",
    maxWidth: "80vw",
    maxHeight: "90vh",
    overflow: "auto",
    borderRadius: radius.floatingPanel,
    backgroundColor: surface.card,
    borderWidth: "var(--control-edge-width)",
    borderStyle: "solid",
    borderColor: surface.field,
    paddingInline: space.s8,
    paddingBottom: space.s8,
    paddingTop: space.s12,
  },
  media: {
    width: "100vw",
    height: "100dvh",
    maxHeight: "none",
    maxWidth: "none",
    overflow: "hidden",
    borderRadius: 0,
    backgroundColor: surface.mediaField,
    boxShadow: "none",
    padding: 0,
  },
  dismiss: { cursor: "zoom-out" },
  srOnly: {
    position: "absolute",
    width: "1px",
    height: "1px",
    padding: 0,
    margin: "-1px",
    overflow: "hidden",
    clipPath: "inset(50%)",
    whiteSpace: "nowrap",
    borderWidth: 0,
  },
});

interface LightboxDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onKeyDown?: KeyboardEventHandler<HTMLDivElement>;
  trigger: ReactElement;
  title: ReactNode;
  children: ReactNode;
  kind?: LightboxKind;
  className?: string;
}

export function LightboxDialog({
  open,
  onOpenChange,
  onKeyDown,
  trigger,
  title,
  children,
  kind = "figure",
  className,
}: LightboxDialogProps) {
  const panel = stylex.props(modalPanel(), styles[kind]);
  const popupRef = useRef<HTMLDivElement>(null);
  return (
    <DialogPrimitive.Root open={open} onOpenChange={onOpenChange}>
      <DialogPrimitive.Trigger render={trigger} />
      <DialogPrimitive.Portal>
        <DialogPrimitive.Backdrop {...stylex.props(MODAL_SCRIM, styles.dismiss)} />
        <DialogPrimitive.Popup
          ref={popupRef}
          initialFocus={popupRef}
          tabIndex={-1}
          aria-describedby={undefined}
          onKeyDown={onKeyDown}
          {...panel}
          className={cn(panel.className, className)}
        >
          <DialogPrimitive.Title {...stylex.props(styles.srOnly)}>{title}</DialogPrimitive.Title>
          {children}
        </DialogPrimitive.Popup>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  );
}
