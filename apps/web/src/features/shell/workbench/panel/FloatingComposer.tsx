import * as stylex from "@stylexjs/stylex";
import { type ReactNode, type Ref, type RefObject } from "react";
import { Slot } from "@/extensions/host/Slot";
import { JumpToBottomButton } from "@/features/shell/workbench/panel/JumpToBottomButton";
import { RuntimeConnectionNotice } from "@/features/shell/workbench/panel/RuntimeConnectionNotice";
import { space } from "@flame/ui/tokens.stylex";
import { readingColumn as rc } from "@/features/chat/public/message/rendering";
const fc = stylex.create({
  tray: {
    display: "flex",
    width: "100%",
    flexShrink: 0,
    flexDirection: "column",
    alignItems: "center",
    paddingTop: { default: null, ":has([data-slot=composer-top-tray-surface])": "1px" },
  },
  overlay: {
    pointerEvents: "none",
    position: "absolute",
    insetInline: 0,
    bottom: 0,
    zIndex: 2,
    display: "flex",
    // Keep the jump control, its gap, and focus clearance inside the conversation.
    maxHeight:
      "calc(100% - max(var(--control-height-md), var(--control-target-min)) - 1rem - var(--notification-composer-clearance, 0px))",
    flexDirection: "column",
  },
  bounded: {
    display: "flex",
    minHeight: 0,
    maxHeight: "var(--notification-empty-composer-limit, none)",
    flexShrink: 0,
    flexDirection: "column",
  },
  holder: {
    pointerEvents: "auto",
    position: "relative",
    display: "flex",
    minHeight: 0,
    flexDirection: "column",
  },
  floor: {
    display: "flex",
    minHeight: 0,
    flexDirection: "column",
  },
  floatingFloor: {
    paddingBottom: { default: space.s3, "@container conversation (min-width: 640px)": space.s4 },
  },
});

function ComposerOverlayTop() {
  return <Slot name="composer.overlay.top" wrapper className={stylex.props(fc.tray).className} />;
}

function ComposerStack({ children }: { children: ReactNode }) {
  return (
    <>
      <RuntimeConnectionNotice />
      <ComposerOverlayTop />
      {children}
    </>
  );
}

export function FloatingComposer({
  overlayRef,
  jumpRef,
  floating,
  children,
}: {
  overlayRef: RefObject<HTMLDivElement | null>;
  jumpRef?: Ref<HTMLDivElement>;
  floating: boolean;
  children: ReactNode;
}) {
  return (
    <div
      ref={overlayRef}
      data-slot="composer-overlay"
      data-floating={floating ? "" : undefined}
      {...stylex.props(rc.box, floating ? fc.overlay : fc.bounded)}
    >
      <div {...stylex.props(rc.gutter, fc.floor, floating && fc.floatingFloor)}>
        <div {...stylex.props(fc.holder)}>
          {floating && <JumpToBottomButton dockRef={jumpRef} />}
          <ComposerStack>{children}</ComposerStack>
        </div>
      </div>
    </div>
  );
}
