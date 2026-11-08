import * as stylex from "@stylexjs/stylex";
import { Dialog } from "@base-ui/react/dialog";
import { X } from "@flame/icons/react";
import {
  use,
  useCallback,
  useId,
  useLayoutEffect,
  useMemo,
  useRef,
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
import { PanelLayoutContext } from "./panel-layout";
import { readingUnitFor } from "./reading-unit";
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
  bodyRef: callerBodyRef,
  bodyStyles,
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
  bodyRef?: RefObject<HTMLDivElement | null>;
  bodyStyles?: stylex.StyleXArray<stylex.StyleXStyles | null | false>;
}) {
  const owner = use(ContentVisibilityContext);
  const portalVisibility = useMemo(
    () => ({ visible: open, fallbackFocus: owner.fallbackFocus }),
    [open, owner.fallbackFocus],
  );
  const titleId = useId();
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
                <Dialog.Title
                  id={titleId}
                  {...stylex.props(styles.title, type.displaySm)}
                >
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
              <DialogBody
                titleId={titleId}
                description={description}
                bodyRef={callerBodyRef}
                bodyStyles={bodyStyles}
              >
                {children}
              </DialogBody>
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

type BodyFrame = {
  top: number;
  bottom: number;
  width: number;
  contentHeight: number;
  unit: Element;
};

function bodyFrame(body: HTMLElement, target: Element): BodyFrame | null {
  if (!body.isConnected || !body.contains(target)) return null;
  const bounds = body.getBoundingClientRect();
  const style = getComputedStyle(body);
  const top = Math.max(
    bounds.top + (Number.parseFloat(style.scrollPaddingBlockStart) || 0),
    0,
  );
  const bottom = Math.min(
    bounds.bottom - (Number.parseFloat(style.scrollPaddingBlockEnd) || 0),
    body.ownerDocument.documentElement.clientHeight,
  );
  const height = bottom - top;
  if (!(height > 0)) return null;
  const unit = readingUnitFor(target, body, (candidate) => {
    const size = candidate.getBoundingClientRect().height;
    return size > 0 && size <= height + 0.5;
  });
  return {
    top,
    bottom,
    width: bounds.width,
    contentHeight: body.scrollHeight,
    unit,
  };
}

function withinBodyFrame(shown: DOMRect, frame: BodyFrame) {
  // Native page zoom reports fractional CSS pixels at a rounded scroll offset.
  return (
    shown.top >= frame.top - 0.5 &&
    (shown.height > frame.bottom - frame.top
      ? shown.top < frame.bottom
      : shown.bottom <= frame.bottom + 0.5)
  );
}

// The portal's React parent may be a page Panel whose viewport does not
// contain this body, so the dialog answers for its own scroll frame.
function DialogBody({
  titleId,
  description,
  bodyRef: callerBodyRef,
  bodyStyles,
  children,
}: {
  titleId: string;
  description?: ReactNode;
  bodyRef?: RefObject<HTMLDivElement | null>;
  bodyStyles?: stylex.StyleXArray<stylex.StyleXStyles | null | false>;
  children?: ReactNode;
}) {
  const bodyRef = useRef<HTMLDivElement>(null);
  const pressedTarget = useRef<Element | null>(null);
  const scrollPosition = useRef(0);
  const attachBody = useCallback(
    (body: HTMLDivElement | null) => {
      bodyRef.current = body;
      if (callerBodyRef) callerBodyRef.current = body;
    },
    [callerBodyRef],
  );
  const keepFocusVisible = useCallback((target: Element) => {
    const body = bodyRef.current;
    const frame = body && bodyFrame(body, target);
    if (!body || !frame) return null;
    const pressed = pressedTarget.current;
    const pointerFocus =
      pressed &&
      (target.contains(pressed) ||
        pressed.closest("label, [data-focus-proxy]")?.contains(target));
    const shown = frame.unit.getBoundingClientRect();
    // Moving a pressed control before pointerup can swallow its activation.
    if (!pointerFocus && !withinBodyFrame(shown, frame)) {
      frame.unit.scrollIntoView({
        block: shown.height > frame.bottom - frame.top ? "start" : "nearest",
        inline: "nearest",
      });
      // Its queued scroll event must not read as the reader leaving.
      scrollPosition.current = body.scrollTop;
    }
    return frame;
  }, []);
  const bodyLayout = useMemo(
    () => ({
      controlsRef: null,
      stickyControlsBounds: () => null,
      keepBodyFocusVisible(target: Element | null) {
        if (target) keepFocusVisible(target);
      },
    }),
    [keepFocusVisible],
  );
  useLayoutEffect(() => {
    const body = bodyRef.current;
    if (!body) return;
    const document = body.ownerDocument;
    // The focused descendant stays revealed until the reader leaves it.
    let focused: Element | null = null;
    let following = false;
    let frame: BodyFrame | null = null;
    scrollPosition.current = body.scrollTop;
    const follow = () => {
      if (!focused || !following) return;
      if (focused !== document.activeElement || !body.contains(focused)) {
        track(null);
        return;
      }
      frame = keepFocusVisible(focused);
    };
    const observer = new ResizeObserver(follow);
    const track = (target: Element | null) => {
      if (focused) observer.unobserve(focused);
      focused = target;
      following = target !== null;
      frame = null;
      // The target's initial observation reveals it after native focus scrolling.
      if (target) observer.observe(target, { box: "border-box" });
    };
    observer.observe(body, { box: "border-box" });
    const initial = document.activeElement;
    if (initial && initial !== body && body.contains(initial)) track(initial);
    const onFocusIn = (event: FocusEvent) => {
      // Window reactivation refocuses the same control without a new entry.
      if (event.target === focused) return;
      track(
        event.target instanceof Element && event.target !== body
          ? event.target
          : null,
      );
    };
    const onFocusOut = (event: FocusEvent) => {
      if (event.target === document.activeElement) return;
      const next = event.relatedTarget;
      if (!(next instanceof Node && body.contains(next)))
        pressedTarget.current = null;
      track(null);
    };
    const onPointerDown = (event: PointerEvent) => {
      pressedTarget.current =
        event.target instanceof Element ? event.target : null;
    };
    const release = () => {
      pressedTarget.current = null;
    };
    const onWheel = () => {
      following = false;
    };
    const onScroll = () => {
      const moved = body.scrollTop !== scrollPosition.current;
      scrollPosition.current = body.scrollTop;
      if (!moved || !focused || !following || !frame) return;
      const current = bodyFrame(body, focused);
      if (!current) return;
      // Reflow and native scroll anchoring can notify before ResizeObserver delivery.
      if (
        current.top !== frame.top ||
        current.bottom !== frame.bottom ||
        current.width !== frame.width ||
        current.contentHeight !== frame.contentHeight
      )
        follow();
      else if (!withinBodyFrame(current.unit.getBoundingClientRect(), current))
        following = false;
    };
    body.addEventListener("focusin", onFocusIn);
    body.addEventListener("focusout", onFocusOut);
    body.addEventListener("pointerdown", onPointerDown, true);
    body.addEventListener("wheel", onWheel, { passive: true });
    body.addEventListener("scroll", onScroll, { passive: true });
    document.addEventListener("pointerup", release, true);
    document.addEventListener("pointercancel", release, true);
    document.defaultView?.addEventListener("resize", follow);
    return () => {
      observer.disconnect();
      body.removeEventListener("focusin", onFocusIn);
      body.removeEventListener("focusout", onFocusOut);
      body.removeEventListener("pointerdown", onPointerDown, true);
      body.removeEventListener("wheel", onWheel);
      body.removeEventListener("scroll", onScroll);
      document.removeEventListener("pointerup", release, true);
      document.removeEventListener("pointercancel", release, true);
      document.defaultView?.removeEventListener("resize", follow);
      release();
    };
  }, [keepFocusVisible]);
  return (
    // A body-filling field leaves later content reachable only by native page scrolling.
    <div
      ref={attachBody}
      role="region"
      aria-labelledby={titleId}
      tabIndex={0}
      data-slot="dialog-body"
      data-focus-inset=""
      {...stylex.props(styles.body, bodyStyles)}
    >
      {description && (
        <Dialog.Description {...stylex.props(styles.description, type.uiMd)}>
          {description}
        </Dialog.Description>
      )}
      <PanelLayoutContext value={bodyLayout}>{children}</PanelLayoutContext>
    </div>
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
  if (!element.isConnected || element.getClientRects().length === 0)
    return false;
  // CSS-hidden boxes keep their layout rectangles but cannot hold focus.
  const { visibility } = getComputedStyle(element);
  return visibility !== "hidden" && visibility !== "collapse";
}
