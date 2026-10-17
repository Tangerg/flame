import * as stylex from "@stylexjs/stylex";
import { DialogPrimitive as Dialog } from "./primitives/dialog";
import { X } from "@flame/icons/react";
import type { ComponentProps, FormEventHandler, ReactNode } from "react";
import { IconButton } from "./icon-button";
import { MODAL_SCRIM, modalPanel } from "./floating-surface";
import {
  color,
  layout,
  radius,
  space,
  surface,
  type,
  weight,
} from "./tokens.stylex";

const styles = stylex.create({
  panel: {
    display: "flex",
    flexDirection: "column",
    width: "min(35rem, calc(100dvw - var(--layout-compact-gutter) * 2))",
    minWidth: 0,
    overflow: "hidden",
    borderRadius: radius.floatingPanel,
    backgroundColor: surface.card,
    color: color.fg,
    overflowWrap: "anywhere",
  },
  small: {
    width: "min(25rem, calc(100dvw - var(--layout-compact-gutter) * 2))",
  },
  large: {
    width: "min(40rem, calc(100dvw - var(--layout-compact-gutter) * 2))",
  },
  header: {
    display: "flex",
    alignItems: "center",
    gap: layout.contentGap,
    padding: layout.cardPadding,
    paddingBlockEnd: layout.fieldGap,
    flexShrink: 0,
    minWidth: 0,
  },
  heading: { display: "grid", gap: layout.controlGap, minWidth: 0, flex: 1 },
  title: { margin: 0, fontWeight: weight.semibold, textWrap: "balance" },
  description: { margin: 0, color: color.fgMuted, textWrap: "pretty" },
  mark: {
    display: "grid",
    placeItems: "center",
    width: space.s9,
    height: space.s9,
    flexShrink: 0,
    borderRadius: radius.lg,
    backgroundColor: surface.canvas,
    color: color.fgMuted,
  },
  body: {
    display: "grid",
    gridTemplateColumns: "minmax(0,1fr)",
    gap: layout.fieldGap,
    minHeight: 0,
    minWidth: 0,
    overflowY: "auto",
    overscrollBehavior: "contain",
    paddingInline: layout.cardPadding,
    paddingBlockEnd: layout.cardPadding,
    scrollPaddingBlock: layout.cardPadding,
  },
  footer: {
    display: "flex",
    alignItems: "center",
    justifyContent: "flex-end",
    flexWrap: "wrap",
    gap: layout.contentGap,
    flexShrink: 0,
    paddingInline: layout.cardPadding,
    paddingBlock: layout.rowPadding,
    backgroundColor: surface.canvas,
  },
  actions: {
    display: "flex",
    alignItems: "center",
    flexWrap: "wrap",
    justifyContent: "flex-end",
    gap: layout.controlGap,
    marginInlineStart: "auto",
    minWidth: 0,
  },
});

export function ModalDialog({
  open,
  onOpenChange,
  title,
  description,
  closeLabel,
  icon,
  children,
  footer,
  auxiliaryAction,
  size = "md",
  initialFocus,
  onSubmit,
  role = "dialog",
  slot,
  backdropSlot,
}: {
  open: boolean;
  onOpenChange(open: boolean): void;
  title: ReactNode;
  description?: ReactNode;
  closeLabel?: string;
  icon?: ReactNode;
  children?: ReactNode;
  footer?: ReactNode;
  auxiliaryAction?: ReactNode;
  size?: "sm" | "md" | "lg";
  initialFocus?: ComponentProps<typeof Dialog.Popup>["initialFocus"];
  onSubmit?: FormEventHandler<HTMLFormElement>;
  role?: "dialog" | "alertdialog";
  slot?: string;
  backdropSlot?: string;
}) {
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Backdrop
          data-slot={backdropSlot}
          {...stylex.props(MODAL_SCRIM)}
        />
        <Dialog.Popup
          data-slot={slot}
          role={role}
          initialFocus={initialFocus}
          render={onSubmit ? <form onSubmit={onSubmit} /> : undefined}
          {...stylex.props(
            modalPanel(),
            styles.panel,
            size === "sm" && styles.small,
            size === "lg" && styles.large,
            type.uiMd,
          )}
        >
          <header {...stylex.props(styles.header)}>
            {icon && <span {...stylex.props(styles.mark)}>{icon}</span>}
            <div {...stylex.props(styles.heading)}>
              <Dialog.Title {...stylex.props(styles.title, type.displaySm)}>
                {title}
              </Dialog.Title>
            </div>
            {closeLabel && (
              <Dialog.Close
                render={
                  <IconButton
                    icon={X}
                    size="sm"
                    quiet
                    aria-label={closeLabel}
                  />
                }
              />
            )}
          </header>
          {(description || children) && (
            <div data-slot="dialog-body" {...stylex.props(styles.body)}>
              {description && (
                <Dialog.Description
                  {...stylex.props(styles.description, type.uiMd)}
                >
                  {description}
                </Dialog.Description>
              )}
              {children}
            </div>
          )}
          {footer && (
            <footer {...stylex.props(styles.footer)}>
              {auxiliaryAction}
              <div {...stylex.props(styles.actions)}>{footer}</div>
            </footer>
          )}
        </Dialog.Popup>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
