import type { DotTone, Tone } from "@/lib/tone";
import type { Translate } from "@/lib/i18n";
import type { AgentRunView } from "@/plugins/sdk/types/agentSessionView";
import {
  agentRunDetail,
  agentRunPresentationState,
  agentRunStepCount,
  type AgentRunPresentationState,
} from "@/plugins/builtin/agent/public/runPresentation";

export interface DelegatedRunCardModel {
  label: string;
  status: AgentRunPresentationState;
  statusLabel: string;
  dotTone: DotTone;
  ink: Tone;
  /** A narrower vocabulary than `ink` on purpose: a card draws an edge around itself only for
   *  something somebody has to act on. */
  shell: Extract<Tone, "neutral" | "warning" | "negative">;
  detail: string | null;
  stepsLabel: string;
  autoExpanded: boolean;
  cancelable: boolean;
}

/**
 * The dot's tone lived here while the status word's ink and the card's own framing were two
 * more ternaries over the same `status` in the JSX — three derivations of one fact, and the
 * kind that drift apart one branch at a time. `ink` and `shell` disagree in exactly one place,
 * which is the distinction worth keeping: a running delegate says "running" in the info ink,
 * and does NOT frame itself, because a run in flight is not a thing to act on.
 */
const STATUS_VIEW: Record<
  AgentRunPresentationState,
  { labelKey: string; dotTone: DotTone; ink: Tone; shell: DelegatedRunCardModel["shell"] }
> = {
  running: {
    labelKey: "agent.runTree.status.running",
    dotTone: "running",
    ink: "info",
    shell: "neutral",
  },
  waiting: {
    labelKey: "agent.runTree.status.waiting",
    dotTone: "waiting",
    ink: "warning",
    shell: "warning",
  },
  finished: {
    labelKey: "agent.runTree.status.finished",
    dotTone: "ok",
    ink: "neutral",
    shell: "neutral",
  },
  error: {
    labelKey: "agent.runTree.status.error",
    dotTone: "err",
    ink: "negative",
    shell: "negative",
  },
  canceled: {
    labelKey: "agent.runTree.status.canceled",
    dotTone: "idle",
    ink: "neutral",
    shell: "neutral",
  },
  limit: {
    labelKey: "agent.runTree.status.limit",
    dotTone: "waiting",
    ink: "warning",
    shell: "warning",
  },
};

export function delegatedRunCardModel(
  t: Translate,
  run: AgentRunView,
  ordinal: number,
  siblingCount: number,
): DelegatedRunCardModel {
  const status = agentRunPresentationState(run);
  const statusView = STATUS_VIEW[status];
  return {
    label:
      siblingCount === 1
        ? t("agent.runTree.delegated.one")
        : t("agent.runTree.delegated.many", { index: ordinal, count: siblingCount }),
    status,
    statusLabel: t(statusView.labelKey),
    dotTone: statusView.dotTone,
    ink: statusView.ink,
    shell: statusView.shell,
    detail: agentRunDetail(run),
    stepsLabel: t("agent.steps", { count: agentRunStepCount(run) }),
    // Exempt from the answer-supersedes-work rule on purpose: a delegated run that is
    // waiting has an interrupt somebody has to act on, and folding away a request for
    // a decision because the parent started talking is how a turn deadlocks in
    // silence. Every other status auto-collapses already.
    autoExpanded: status === "waiting",
    cancelable: run.status !== "finished",
  };
}
