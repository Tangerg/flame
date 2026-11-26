import { type AgentAccess, useAgent } from "@/features/agent/public/agent";
import { useWorkspace } from "@/features/workspace/public/workspace";
import { iconGlyph } from "@/ui/icons/index";
import * as stylex from "@stylexjs/stylex";
import { useEffect, useState } from "react";
import { AnimatePresence, motion } from "motion/react";
import { Icon } from "@/ui/index";
import { IconButton, vocab } from "@flame/ui";
import { BannerAction } from "@/features/shell/workbench/panel/BannerAction";
import { flattenText } from "@/features/agent/public/messageContent";

import { agentTextInput } from "@/features/agent/public/input";
import { useCanSendToAgent, useChatSend } from "@/features/agent/public/input";

import { useActiveSessionProblem } from "@/features/agent/public/run";
import { useT, type Translate } from "@/lib/i18n/index";
import { useMotion } from "@/lib/motion";
import { describeActiveRunRefusal, describeErrorType } from "@/lib/rpcErrors";
import { openWorkspaceClientDiagnostics } from "@/features/workspace/public/deeplinks";
import { showWorkspaceDock } from "@/features/workspace/public/navigation";
import { useRuntimeCommandsAvailable } from "@/features/connection/public/serviceStatus";
import { type AgentProblem } from "@/features/agent/public/sessionView";
import { shellStyles as sh } from "@/features/shell/workbench/shellStyles";
import { color, font, space, type as typeStep } from "@flame/ui/tokens.stylex";
const reb = stylex.create({
  sans: { fontFamily: font.sans },
  glyph: { marginTop: space.s0_5, color: color.negative },
});

function findLastUserText(agentAccess: AgentAccess): string {
  const last = agentAccess
    .getActiveConversationMessages()
    .findLast((m) => m.role === "user" && flattenText(m.blocks).trim() !== "");
  return last ? flattenText(last.blocks).trim() : "";
}

interface RetryCountdown {
  problem: AgentProblem | null;
  retryAfter: number;
  remaining: number;
}

// Seconds left before a rate-limited Run may be retried, counted from when the
// problem arrived.
function useRetryCountdown(error: AgentProblem | null): number {
  const retryAfter = error?.retryAfterSeconds ?? 0;
  const [countdown, setCountdown] = useState<RetryCountdown>({
    problem: null,
    retryAfter: 0,
    remaining: 0,
  });
  useEffect(() => {
    if (retryAfter <= 0) return;
    const started = performance.now();
    const id = setInterval(() => {
      const rem = Math.max(0, Math.ceil(retryAfter - (performance.now() - started) / 1000));
      setCountdown({ problem: error, retryAfter, remaining: rem });
      if (rem <= 0) clearInterval(id);
    }, 250);
    return () => clearInterval(id);
  }, [error, retryAfter]);
  return countdown.problem === error && countdown.retryAfter === retryAfter
    ? countdown.remaining
    : retryAfter;
}

// Retrying cannot help a stopped Run or one that needs new credentials.
function retryRecovers(error: AgentProblem | null | undefined): boolean {
  return error?.recovery !== "stop" && error?.recovery !== "reauthenticate";
}

// The localized cause leads; a provider's own message, when it adds something, follows it.
function runProblemText(
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

export function RunErrorBanner() {
  const { disclosureExitTransition, disclosureTransition } = useMotion();
  const agentAccess = useAgent();
  const workspaceAccess = useWorkspace();

  const t = useT();
  const error = useActiveSessionProblem();
  const send = useChatSend();
  const hasSendAction = useCanSendToAgent();
  const runtimeAvailable = useRuntimeCommandsAvailable();
  const canSend = hasSendAction && runtimeAvailable;

  const retryIn = useRetryCountdown(error);

  const retryText = error ? findLastUserText(agentAccess) : "";

  const onRetry = () => {
    if (retryIn > 0 || !canSend || !retryText) return;
    if (!send(agentTextInput(retryText))) return;
    agentAccess.dismissActiveSessionProblem();
  };

  const canRetry = canSend && Boolean(retryText) && retryRecovers(error);
  const problem = error ? runProblemText(t, error) : null;

  return (
    <AnimatePresence initial={false}>
      {error && (
        <motion.div
          role="alert"
          initial={{ opacity: 0, y: 6 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0, y: 6, transition: disclosureExitTransition }}
          transition={disclosureTransition}
          className={stylex.props(sh.bannerCard, reb.sans, vocab.ink).className}
        >
          <Icon name="alert" size="md" className={stylex.props(reb.glyph).className} />
          <div {...stylex.props(vocab.min)}>
            <div {...stylex.props(sh.bannerBody, vocab.strong, typeStep.uiMd)}>
              {problem?.headline}
            </div>
            {problem?.detail && (
              <div {...stylex.props(sh.bannerBody, sh.selectable, vocab.muted, typeStep.uiSm)}>
                {problem.detail}
              </div>
            )}
            <div {...stylex.props(sh.bannerActions)}>
              {canRetry && (
                <BannerAction
                  icon="loop"
                  label={
                    retryIn > 0
                      ? t("runError.action.retryIn", { seconds: retryIn })
                      : t("common.retry")
                  }
                  onClick={onRetry}
                  disabled={retryIn > 0}
                  primary
                />
              )}
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
            onClick={agentAccess.dismissActiveSessionProblem}
          />
        </motion.div>
      )}
    </AnimatePresence>
  );
}
