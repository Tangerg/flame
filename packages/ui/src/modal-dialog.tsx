import * as stylex from "@stylexjs/stylex";
import { DialogPrimitive as Dialog } from "./primitives/dialog";
import { X } from "@flame/icons/react";
import {
  use,
  useMemo,
  type ComponentProps,
  type FormEventHandler,
  type ReactNode,
  type RefObject,
} from "react";
import { ContentVisibilityContext } from "./content-visibility";
import { IconButton } from "./icon-button";
import { IconMark } from "./icon-mark";
import { isImeKey } from "./keyboard";
import { MODAL_SCRIM, modalPanel } from "./floating-surface";
import { color, layout, surface, type, weight } from "./tokens.stylex";

const styles = stylex.create({
  portal: { position: "relative", zIndex: "var(--layer-modal)" },
  backdrop: { zIndex: 0 },
  panel: {
    zIndex: 1,
    display: "flex",
    flexDirection: "column",
    width: "min(35rem, calc(100dvw - var(--layout-compact-gutter) * 2))",
    minWidth: 0,
    overflow: "hidden",
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
  heading: { minWidth: 0, flex: 1 },
  title: { margin: 0, fontWeight: weight.semibold, textWrap: "balance" },
  description: { margin: 0, color: color.fgMuted, textWrap: "pretty" },
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
  finalFocus,
  dismissible = true,
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
  finalFocus?: ComponentProps<typeof Dialog.Popup>["finalFocus"];
  dismissible?: boolean;
  onSubmit?: FormEventHandler<HTMLFormElement>;
  role?: "dialog" | "alertdialog";
  slot?: string;
  backdropSlot?: string;
}) {
  const owner = use(ContentVisibilityContext);
  const portalVisibility = useMemo(
    () => ({ visible: open, fallbackFocus: owner.fallbackFocus }),
    [open, owner.fallbackFocus],
  );
  return (
    <Dialog.Root
      open={open}
      onOpenChange={(next, details) => {
        // Base UI checks only isComposing; WebKit reports some IME keys solely
        // as keyCode 229. Canceling leaves the composition's own Escape intact.
        if (
          !next &&
          (!dismissible ||
            (details.reason === "escape-key" && isImeKey(details.event)))
        ) {
          details.cancel();
          return;
        }
        onOpenChange(next);
      }}
    >
      <Dialog.Portal {...stylex.props(styles.portal)}>
        <ContentVisibilityContext value={portalVisibility}>
          <Dialog.Backdrop
            data-slot={backdropSlot}
            {...stylex.props(MODAL_SCRIM, styles.backdrop)}
          />
          <Dialog.Popup
            data-slot={slot}
            role={role}
            initialFocus={initialFocus}
            finalFocus={
              owner.visible
                ? finalFocus
                : (closeType) =>
                    hiddenOwnerFinalFocus(
                      finalFocus,
                      closeType,
                      owner.fallbackFocus,
                    )
            }
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
              {icon && <IconMark>{icon}</IconMark>}
              <div {...stylex.props(styles.heading)}>
                <Dialog.Title {...stylex.props(styles.title, type.displaySm)}>
                  {title}
                </Dialog.Title>
              </div>
              {closeLabel && (
                <Dialog.Close
                  disabled={!dismissible}
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
            {(footer || auxiliaryAction) && (
              <footer {...stylex.props(styles.footer)}>
                {auxiliaryAction}
                {footer && (
                  <div {...stylex.props(styles.actions)}>{footer}</div>
                )}
              </footer>
            )}
          </Dialog.Popup>
        </ContentVisibilityContext>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

type FinalFocus = ComponentProps<typeof Dialog.Popup>["finalFocus"];
type CloseType = Parameters<
  Extract<FinalFocus, (...args: never[]) => unknown>
>[0];

// Base UI's default return target is the opener, which lives in the hidden
// owner; focusing it would land in content the user cannot see.
function hiddenOwnerFinalFocus(
  finalFocus: FinalFocus,
  closeType: CloseType,
  fallbackFocus: RefObject<HTMLElement | null> | undefined,
): HTMLElement | false | undefined {
  if (finalFocus === false) return false;
  let target: HTMLElement | null = null;
  if (typeof finalFocus === "function") {
    const resolved = finalFocus(closeType);
    if (resolved === undefined) return undefined;
    if (resolved === false) return false;
    if (resolved instanceof HTMLElement) target = resolved;
  } else if (finalFocus && finalFocus !== true) {
    target = finalFocus.current;
  }
  if (target && isShown(target)) return target;
  return fallbackFocus?.current ?? false;
}

function isShown(element: HTMLElement): boolean {
  return element.isConnected && element.getClientRects().length > 0;
}
