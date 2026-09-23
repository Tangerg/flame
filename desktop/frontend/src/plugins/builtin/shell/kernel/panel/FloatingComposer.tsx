import * as stylex from "@stylexjs/stylex";
import type { ReactNode, RefObject } from "react";
import { AnimatePresence, motion } from "motion/react";
import { useT } from "@/lib/i18n";
import { disclosureExitTransition, disclosureTransition } from "@/lib/motion";
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
  tray: {
    display: "flex",
    width: "100%",
    flexDirection: "column",
    alignItems: "center",
    paddingTop: { default: null, ":has([data-slot=composer-top-tray-surface])": "1px" },
  },
  overlay: { pointerEvents: "none", position: "absolute", insetInline: 0, bottom: 0, zIndex: 2 },
  holder: { pointerEvents: "auto", position: "relative" },
  floor: {
    paddingBottom: { default: space.s3, "@container conversation (min-width: 640px)": space.s4 },
  },
});

function RuntimeConnectionNotice() {
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
          exit={{ opacity: 0, y: 2, transition: disclosureExitTransition }}
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

function ComposerOverlayTop() {
  return <Slot name="composer.overlay.top" wrapper className={stylex.props(fc.tray).className} />;
}

export function ComposerStack({ children }: { children: ReactNode }) {
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
          <ComposerStack>{children}</ComposerStack>
        </div>
      </div>
    </div>
  );
}
