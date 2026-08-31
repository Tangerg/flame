import type { ReactNode, RefObject } from "react";
import { AnimatePresence, motion } from "motion/react";
import { cn } from "@/lib/classNames";
import { useT } from "@/lib/i18n";
import { disclosureTransition } from "@/lib/motion";
import { useRuntimeServiceStatus } from "@/plugins/builtin/runtime/public/serviceStatus";
import { CONNECTION_PANE } from "@/plugins/builtin/settings/kit/panes";
import { openWorkspaceSettingsPane } from "@/plugins/builtin/workspace/public/navigation";
import { Slot } from "@/plugins/host/Slot";
import { SystemMessage } from "@/ui";
import { JumpToBottomButton } from "./JumpToBottomButton";
import { READING_COLUMN, READING_GUTTER } from "./readingColumn";

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
          className="mb-2"
        >
          <SystemMessage
            variant={unavailable ? "error" : "warning"}
            icon={unavailable ? "alert" : "loop"}
            role={unavailable ? "alert" : "status"}
            aria-live={unavailable ? "assertive" : "polite"}
            className="text-pretty"
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
  return (
    <Slot
      name="composer.overlay.top"
      wrapper
      className="flex w-full flex-col items-center [&:has([data-slot=composer-top-tray-surface])]:pt-px"
    />
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
    <div
      ref={overlayRef}
      className={cn("pointer-events-none absolute inset-x-0 bottom-0 z-2", READING_COLUMN)}
    >
      <div className={cn(READING_GUTTER, "pb-3 sm:pb-4")}>
        <div className="pointer-events-auto relative">
          <JumpToBottomButton />
          <ComposerOverlayTop />
          <RuntimeConnectionNotice />
          {children}
        </div>
      </div>
    </div>
  );
}
