import type { DotTone, Tone } from "@/lib/tone";
import type { Translate } from "@/lib/i18n";
import type { AgentRunView } from "@/plugins/sdk/types/agentSessionView";
import {
  agentRunDetail,
  agentRunPresentationState,
  type AgentRunPresentationState,
} from "@/plugins/builtin/agent/public/runPresentation";

export interface DelegatedRunSummary {
  label: string;
  statusLabel: string;
  dotTone: DotTone;
  ink: Tone;
  detail: string | null;
  cancelable: boolean;
}

const STATUS_VIEW: Record<
  AgentRunPresentationState,
  { labelKey: string; dotTone: DotTone; ink: Tone }
> = {
  running: {
    labelKey: "agent.runTree.status.running",
    dotTone: "running",
    ink: "info",
  },
  waiting: {
    labelKey: "agent.runTree.status.waiting",
    dotTone: "waiting",
    ink: "warning",
  },
  finished: {
    labelKey: "agent.runTree.status.finished",
    dotTone: "ok",
    ink: "neutral",
  },
  error: {
    labelKey: "agent.runTree.status.error",
    dotTone: "err",
    ink: "negative",
  },
  canceled: {
    labelKey: "agent.runTree.status.canceled",
    dotTone: "idle",
    ink: "neutral",
  },
  limit: {
    labelKey: "agent.runTree.status.limit",
    dotTone: "waiting",
    ink: "warning",
  },
};

export function delegatedRunSummary(
  t: Translate,
  run: AgentRunView,
  ordinal: number,
  siblingCount: number,
): DelegatedRunSummary {
  const status = agentRunPresentationState(run);
  const statusView = STATUS_VIEW[status];
  return {
    label:
      siblingCount === 1
        ? t("agent.runTree.delegated.one")
        : t("agent.runTree.delegated.many", { index: ordinal, count: siblingCount }),
    statusLabel: t(statusView.labelKey),
    dotTone: statusView.dotTone,
    ink: statusView.ink,
    detail: agentRunDetail(run),
    cancelable: run.status !== "finished",
  };
}
