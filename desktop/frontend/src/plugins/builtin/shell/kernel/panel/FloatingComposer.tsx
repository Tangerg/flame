import * as stylex from "@stylexjs/stylex";
import type { ReactNode, RefObject } from "react";
import { AnimatePresence, motion } from "motion/react";
import { useT } from "@/lib/i18n";
import { disclosureTransition } from "@/lib/motion";
import { useRuntimeServiceStatus } from "@/plugins/builtin/runtime/public/serviceStatus";
import { CONNECTION_PANE } from "@/plugins/builtin/settings/kit/panes";
import { openWorkspaceSettingsPane } from "@/plugins/builtin/workspace/public/navigation";
import { Slot } from "@/plugins/host/Slot";
import { SystemMessage, vocab } from "@/ui";
import { JumpToBottomButton } from "./JumpToBottomButton";
import { space } from "@/styles/tokens.stylex";
import { readingColumn as rc } from "./readingColumn";

const fc = stylex.create({
  notice: { marginBottom: space.s2 },
  // The tray's own top pixel appears only when something is IN it — a `:has()` on itself,
  // which is a condition on this element and so does have a StyleX form.
  tray: {
    display: "flex",
    width: "100%",
    flexDirection: "column",
    alignItems: "center",
    paddingTop: { default: null, ":has([data-slot=composer-top-tray-surface])": "1px" },
  },
  // Transparent to the pointer so the transcript scrolls under it; the composer inside is not.
  overlay: { pointerEvents: "none", position: "absolute", insetInline: 0, bottom: 0, zIndex: 2 },
  holder: { pointerEvents: "auto", position: "relative" },
  floor: { paddingBottom: { default: space.s3, "@media (min-width: 640px)": space.s4 } },
});

export function RuntimeConnectionNotice() {
  const t = useT();
  const service = useRuntimeServiceStatus();
  const visible = service.phase === "reconnecting" || service.phase === "unavailable";
  const unavailable = service.phase === "unavailable";

  return (
    <AnimatePresence initial={false}>
      {visible && (
        <motion.div
          key="runtime-connection-notice"
          initial={{ opacity: 0, y: 4 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0, y: 2 }}
          transition={disclosureTransition}
          className={stylex.props(fc.notice).className}
        >
          <SystemMessage
            variant={unavailable ? "error" : "warning"}
            icon={unavailable ? "alert" : "loop"}
            role={unavailable ? "alert" : "status"}
            aria-live={unavailable ? "assertive" : "polite"}
            className={stylex.props(vocab.pretty).className}
            action={
              unavailable
                ? {
                    label: t("runtime.connection.settings"),
                    onClick: () => openWorkspaceSettingsPane(CONNECTION_PANE),
                  }
                : undefined
            }
          >
            {t(unavailable ? "runtime.connection.unavailable" : "runtime.connection.reconnecting")}
          </SystemMessage>
        </motion.div>
      )}
    </AnimatePresence>
  );
}

export function ComposerOverlayTop() {
  return <Slot name="composer.overlay.top" wrapper className={stylex.props(fc.tray).className} />;
}

export function FloatingComposer({
  overlayRef,
  children,
}: {
  overlayRef: RefObject<HTMLDivElement | null>;
  children: ReactNode;
}) {
  return (
    <div ref={overlayRef} {...stylex.props(fc.overlay, rc.box)}>
      <div {...stylex.props(rc.gutter, fc.floor)}>
        <div {...stylex.props(fc.holder)}>
          <JumpToBottomButton />
          <ComposerOverlayTop />
          <RuntimeConnectionNotice />
          {children}
        </div>
      </div>
    </div>
  );
}
