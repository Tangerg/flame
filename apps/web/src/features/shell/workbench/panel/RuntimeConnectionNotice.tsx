import * as stylex from "@stylexjs/stylex";
import { AnimatePresence, motion } from "motion/react";
import { useWorkspace } from "@/features/workspace/public/workspace";
import { useT } from "@/lib/i18n/index";
import { useMotion } from "@/lib/motion";
import { useRuntimeServiceStatus } from "@/features/connection/public/serviceStatus";
import { CONNECTION_PANE } from "@/features/settings/public/paneIDs";
import { openWorkspaceSettingsPane } from "@/features/workspace/public/navigation";
import { SystemMessage } from "@/ui/index";
import { vocab } from "@flame/ui";
import { space } from "@flame/ui/tokens.stylex";

const styles = stylex.create({
  notice: { flexShrink: 0, marginBottom: space.s2 },
});

export function RuntimeConnectionNotice() {
  const { disclosureExitTransition, disclosureTransition } = useMotion();
  const workspaceAccess = useWorkspace();
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
          exit={{ opacity: 0, y: 4, transition: disclosureExitTransition }}
          transition={disclosureTransition}
          className={stylex.props(styles.notice).className}
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
                    onClick: () => openWorkspaceSettingsPane(workspaceAccess, CONNECTION_PANE),
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
