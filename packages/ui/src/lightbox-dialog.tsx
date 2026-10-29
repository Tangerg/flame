import * as stylex from "@stylexjs/stylex";
import {
  useEffectEvent,
  useLayoutEffect,
  useRef,
  type ComponentProps,
  type KeyboardEventHandler,
  type ReactElement,
  type ReactNode,
} from "react";
import { clsx as cn } from "clsx";
import { X } from "@flame/icons/react";
import { layout, space, surface, type, weight } from "./tokens.stylex";
import { Dialog as DialogPrimitive } from "@base-ui/react/dialog";
import { IconButton } from "./icon-button";
import { ScrollRegion } from "./scroll-region";
import { useContentVisible } from "./content-visibility";
import { MODAL_SCRIM, modalPanel } from "./floating-surface";
const styles = stylex.create({
  figure: {
    width: "fit-content",
    maxHeight: "90vh",
    maxWidth: "min(1400px, 95vw)",
    overflow: "auto",
    padding: space.s6,
  },
  document: {
    display: "flex",
    flexDirection: "column",
    width: "fit-content",
    minWidth: "min(408px, 80vw)",
    maxWidth: "80vw",
    maxHeight: "90vh",
    overflow: "hidden",
    padding: 0,
  },
  documentHeader: {
    display: "flex",
    alignItems: "center",
    gap: layout.contentGap,
    minWidth: 0,
    flexShrink: 0,
    paddingInline: layout.cardPadding,
    paddingBlockStart: layout.cardPadding,
    paddingBlockEnd: layout.fieldGap,
  },
  documentTitle: {
    flex: 1,
    minWidth: 0,
    margin: 0,
    fontWeight: weight.semibold,
    overflowWrap: "anywhere",
    textWrap: "balance",
  },
  documentBody: {
    minHeight: 0,
    overscrollBehavior: "contain",
    paddingInline: layout.cardPadding,
    paddingBlockStart: 0,
    paddingBlockEnd: layout.cardPadding,
    scrollPaddingBlock: layout.controlGap,
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

interface LightboxBaseProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onKeyDown?: KeyboardEventHandler<HTMLDivElement>;
  finalFocus?: ComponentProps<typeof DialogPrimitive.Popup>["finalFocus"];
  trigger: ReactElement;
  title: ReactNode;
  children: ReactNode;
  className?: string;
  styles?: stylex.StyleXArray<stylex.StyleXStyles | null | false>;
}

export type LightboxDialogProps = LightboxBaseProps &
  (
    | { kind?: "figure" | "media" }
    | { kind: "document"; closeLabel: string; contentLabel: string }
  );

export function LightboxDialog(props: LightboxDialogProps) {
  const {
    open,
    onOpenChange,
    onKeyDown,
    finalFocus,
    trigger,
    title,
    children,
    className,
    styles: extra,
  } = props;
  const kind = props.kind ?? "figure";
  const panel = stylex.props(modalPanel(), styles[kind], extra);
  const popupRef = useRef<HTMLDivElement>(null);
  const documentRef = useRef<HTMLDivElement>(null);
  const visible = useContentVisible();
  const retired = open && !visible;
  const retire = useEffectEvent(() => {
    const popup = popupRef.current;
    const active = popup?.ownerDocument.activeElement;
    // Release owned portal focus before the host delivers its new presentation.
    if (active instanceof HTMLElement && popup?.contains(active)) active.blur();
    onOpenChange(false);
  });
  useLayoutEffect(() => {
    if (retired) retire();
  }, [retired]);
  return (
    <DialogPrimitive.Root open={open && visible} onOpenChange={onOpenChange}>
      <DialogPrimitive.Trigger render={trigger} />
      <DialogPrimitive.Portal>
        <DialogPrimitive.Backdrop
          {...stylex.props(MODAL_SCRIM, styles.dismiss)}
        />
        <DialogPrimitive.Popup
          ref={popupRef}
          initialFocus={kind === "document" ? documentRef : popupRef}
          finalFocus={visible ? finalFocus : false}
          tabIndex={-1}
          aria-describedby={undefined}
          onKeyDown={onKeyDown}
          {...panel}
          className={cn(panel.className, className)}
        >
          {props.kind === "document" ? (
            <>
              <div {...stylex.props(styles.documentHeader)}>
                <DialogPrimitive.Title
                  {...stylex.props(styles.documentTitle, type.displaySm)}
                >
                  {title}
                </DialogPrimitive.Title>
                <DialogPrimitive.Close
                  render={
                    <IconButton
                      icon={X}
                      size="sm"
                      quiet
                      aria-label={props.closeLabel}
                    />
                  }
                />
              </div>
              <ScrollRegion
                ref={documentRef}
                label={props.contentLabel}
                data-focus-inset=""
                styles={styles.documentBody}
              >
                {children}
              </ScrollRegion>
            </>
          ) : (
            <>
              <DialogPrimitive.Title {...stylex.props(styles.srOnly)}>
                {title}
              </DialogPrimitive.Title>
              {children}
            </>
          )}
        </DialogPrimitive.Popup>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  );
}
