import * as stylex from "@stylexjs/stylex";
import { useEffect, useState } from "react";
import { AnimatePresence, motion } from "motion/react";
import { Icon, IconButton } from "@/ui";
import { BannerAction } from "./BannerAction";
import { flattenText } from "@/plugins/builtin/agent/public/messageContent";
import { getActiveConversationSnapshot } from "@/plugins/builtin/agent/public/conversation";
import {
  agentTextInput,
  useCanSendToAgent,
  useChatSend,
} from "@/plugins/builtin/agent/public/input";
import {
  dismissActiveSessionProblem,
  useActiveSessionProblem,
} from "@/plugins/builtin/agent/public/run";
import { useT } from "@/lib/i18n";
import { disclosureTransition } from "@/lib/motion";
import { describeErrorType } from "@/lib/rpcErrors";
import {
  openDiagnosticsView,
  openTimelineView,
} from "@/plugins/builtin/workspace/public/deeplinks";
import { useRuntimeCommandsAvailable } from "@/plugins/builtin/runtime/public/serviceStatus";
import type { AgentProblem } from "@/plugins/sdk/types/agentSessionView";
import { shellStyles as sh } from "../shellStyles";
import { color, face, space, type as typeStep, weight } from "@/styles/tokens.stylex";

const reb = stylex.create({
  plain: { fontWeight: weight.regular },
  sans: { fontFamily: "var(--font-sans)" },
  // On the first line of the text beside it, not on the card's top edge.
  glyph: { marginTop: space.s0_5, color: color.negative },
});

function findLastUserText(): string {
  const { messages } = getActiveConversationSnapshot();
  const last = messages.findLast((m) => m.role === "user" && flattenText(m.blocks).trim() !== "");
  return last ? flattenText(last.blocks).trim() : "";
}

const UNRETRYABLE: readonly string[] = ["invalid_api_key", "invalid_params", "provider_rejected"];

interface RetryCountdown {
  problem: AgentProblem | null;
  retryAfter: number;
  remaining: number;
}

export function RunErrorBanner() {
  const t = useT();
  const error = useActiveSessionProblem();
  const send = useChatSend();
  const hasSendAction = useCanSendToAgent();
  const runtimeAvailable = useRuntimeCommandsAvailable();
  const canSend = hasSendAction && runtimeAvailable;

  const retryAfter = error?.retryAfterSeconds ?? 0;
  const [countdown, setCountdown] = useState<RetryCountdown>({
    problem: null,
    retryAfter: 0,
    remaining: 0,
  });
  const retryIn =
    countdown.problem === error && countdown.retryAfter === retryAfter
      ? countdown.remaining
      : retryAfter;
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

  const retryText = error ? findLastUserText() : "";

  const onRetry = () => {
    if (retryIn > 0 || !canSend || !retryText) return;
    if (!send(agentTextInput(retryText))) return;
    dismissActiveSessionProblem();
  };

  const canRetry = canSend && Boolean(retryText) && !UNRETRYABLE.includes(error?.code ?? "");

  return (
    <AnimatePresence initial={false}>
      {error && (
        <motion.div
          role="alert"
          initial={{ opacity: 0, y: -6 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0, y: -6 }}
          transition={disclosureTransition}
          className={stylex.props(sh.banner, sh.bannerCard, reb.sans, sh.ink).className}
        >
          <Icon name="alert" size="sm" className={stylex.props(reb.glyph).className} />
          <div {...stylex.props(sh.min)}>
            <div {...stylex.props(sh.bannerHead)}>
              <span {...stylex.props(sh.negative, sh.strong, typeStep.uiMd)}>
                {t("runError.title")}
              </span>
              {error.code && (
                <span {...stylex.props(sh.faint, sh.selectable, typeStep.uiXs, face.mono)}>
                  {error.code}
                </span>
              )}
            </div>
            <div {...stylex.props(sh.bannerBody, typeStep.uiMd)}>
              {error.message ?? describeErrorType(error.code) ?? t("runError.unknown")}
            </div>
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
                icon="history"
                label={t("runError.action.timeline")}
                onClick={openTimelineView}
              />
              <BannerAction
                icon="spark"
                label={t("runError.action.diagnostics")}
                onClick={openDiagnosticsView}
              />
            </div>
          </div>
          <IconButton
            icon="x"
            iconSize="xs"
            size="xs"
            quiet
            title={t("runError.action.dismiss")}
            onClick={dismissActiveSessionProblem}
          />
        </motion.div>
      )}
    </AnimatePresence>
  );
}
