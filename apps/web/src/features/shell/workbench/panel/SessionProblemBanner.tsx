import { useWorkspace } from "@/features/workspace/public/workspace";
import { iconGlyph } from "@/ui/icons/index";
import * as stylex from "@stylexjs/stylex";
import { AnimatePresence, motion } from "motion/react";
import { Icon } from "@/ui/index";
import { IconButton, vocab } from "@flame/ui";
import { BannerAction } from "@/features/shell/workbench/panel/BannerAction";
import { useActiveSessionProblem } from "@/features/agent/public/run";
import { useT, type Translate } from "@/lib/i18n/index";
import { useMotion } from "@/lib/motion";
import { describeActiveRunRefusal, describeErrorType } from "@/lib/rpcErrors";
import { openWorkspaceClientDiagnostics } from "@/features/workspace/public/deeplinks";
import { showWorkspaceDock } from "@/features/workspace/public/navigation";
import { type AgentProblem } from "@/features/agent/public/sessionView";
import { shellStyles as sh } from "@/features/shell/workbench/shellStyles";
import { color, font, space, type as typeStep } from "@flame/ui/tokens.stylex";
const styles = stylex.create({
  sans: { fontFamily: font.sans },
  glyph: { marginTop: space.s0_5, color: color.negative },
});

function describeSessionProblem(
  t: Translate,
  error: AgentProblem,
): { headline: string; detail: string | undefined } {
  const named =
    describeActiveRunRefusal(error) ??
    (error.lossReason ? t(`agent.loss.${error.lossReason}`) : undefined) ??
    describeErrorType(error.code);
  if (!named) return { headline: error.message ?? t("runError.unknown"), detail: undefined };
  return { headline: named, detail: error.message !== named ? error.message : undefined };
}

export function SessionProblemBanner() {
  const { disclosureExitTransition, disclosureTransition } = useMotion();
  const workspaceAccess = useWorkspace();

  const t = useT();
  const notice = useActiveSessionProblem();
  const problem = notice && { ...describeSessionProblem(t, notice.error), dismiss: notice.dismiss };

  return (
    <AnimatePresence initial={false}>
      {problem && (
        <motion.div
          role="alert"
          initial={{ opacity: 0, y: 6 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0, y: 6, transition: disclosureExitTransition }}
          transition={disclosureTransition}
          className={stylex.props(sh.bannerCard, styles.sans, vocab.ink).className}
        >
          <Icon name="alert" size="md" className={stylex.props(styles.glyph).className} />
          <div {...stylex.props(vocab.min)}>
            <div {...stylex.props(sh.bannerBody, vocab.strong, typeStep.uiMd)}>
              {problem.headline}
            </div>
            {problem.detail && (
              <div {...stylex.props(sh.bannerBody, sh.selectable, vocab.muted, typeStep.uiSm)}>
                {problem.detail}
              </div>
            )}
            <div {...stylex.props(sh.bannerActions)}>
              <BannerAction
                label={t("runError.action.context")}
                onClick={(event) =>
                  showWorkspaceDock(workspaceAccess, undefined, event.currentTarget)
                }
              />
              <BannerAction
                label={t("runError.action.diagnostics")}
                onClick={(event) =>
                  openWorkspaceClientDiagnostics(workspaceAccess, event.currentTarget)
                }
              />
            </div>
          </div>
          <IconButton
            icon={iconGlyph("x")}
            size="xs"
            quiet
            aria-label={t("runError.action.dismiss")}
            onClick={problem.dismiss}
          />
        </motion.div>
      )}
    </AnimatePresence>
  );
}
