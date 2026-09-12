import * as stylex from "@stylexjs/stylex";
import { useRef, type KeyboardEventHandler, type ReactElement, type ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { radius, space, surface } from "@/styles/tokens.stylex";
import { DialogPrimitive } from "@/ui/primitives";
import { MODAL_SCRIM, modalPanel } from "./floating-surface";

/**
 * What the lightbox is holding, which is the only thing that varies between its three uses.
 *
 * `figure` fits the thing and caps it at what the viewport allows. `document` holds a readable
 * measure and scrolls. `media` gives the whole screen to an image on a near-black field — so it
 * has no corner to round and nothing to cast a shadow onto, which is why its call site had been
 * cancelling seven of the panel's properties one by one. Under Tailwind that worked; under
 * StyleX every one of them was discarded and the dialog collapsed to its content.
 */
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
    // A border on top of `--shadow-modal`, which already carries a ring — the double edge
    // DESIGN.md §5 forbids. Preserved as it was; whether the ring or the line goes is a design
    // decision, not a migration.
    borderWidth: "1px",
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
  // Focus the PANEL on open, not the first tabbable control in it.
  //
  // Base UI's default is the first tabbable element, and in a lightbox that is an icon button
  // with a tip — so opening one put focus on a control the person never navigated to, its
  // tooltip opened, and the tooltip then sat above the dialog on the dismiss stack and ate the
  // first Escape. Measured: the image gallery focused "Download image", and Escape #1 closed
  // that tip while the dialog stayed; Escape #2 closed the dialog. Pressing Escape once, which
  // is the whole contract, appeared to do nothing.
  //
  // Focusing the panel lets the dialog announce its own title; the first Tab reaches its
  // controls. Content-specific keys belong to this popup, including before the first Tab:
  // the modal boundary stops composite keys from reaching listeners outside the dialog.
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
