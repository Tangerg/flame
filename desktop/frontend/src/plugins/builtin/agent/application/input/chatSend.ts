import type { AgentRunStartOptions } from "@/plugins/sdk";
import { useCallback } from "react";
import { t } from "@/lib/i18n";
import { describeRpcError } from "@/lib/rpcErrors";
import { notifyError } from "@/plugins/sdk";
import { resolveAgentRunStartOptions } from "@/plugins/sdk";
import type { AgentInput } from "../../domain/input";
import { OPTIMISTIC_STEER_MESSAGE_PREFIX } from "../view/optimisticMessageIdentity";
import { agentRuntime } from "../ports/runtimeGateway";
import { agentSessionView } from "../ports/sessionView";
import { getActiveSessionId, useActiveSessionId } from "../session/activeSession";
import { selectCurrentRootRun } from "../view/runTree";
import { agentCommandOwner } from "../agentCommandOwner";
import { useCurrentRootMaterial } from "../run/runReadModel";
import { ExactSequence } from "@/foundation/exactSequence";

type SendToAgent = (input: AgentInput, options?: AgentRunStartOptions) => boolean;
/**
 * The single send entry point, so Enter and the Send button cannot diverge. With a run
 * already streaming this STEERS it rather than opening a turn: the message renders
 * optimistically and the fold reconciles it by content when the runtime drains the steer. A
 * run that finished between typing and sending (`run_not_found`) rolls the bubble back and
 * opens a fresh turn, so it is never lost and never duplicated.
 */
export function useChatSend(): (input: AgentInput) => boolean {
  const send = agentSessionView().useAction("send");
  return useCallback(
    (input: AgentInput) => {
      const sessionId = getActiveSessionId();
      const runOptions = resolveAgentRunStartOptions();
      // Admission is decided at event time, not from the render that created
      // this callback. A Run can park for HITL between the last paint and an
      // Enter keydown; steering a captured `running` identity would clear the
      // composer before the Runtime rejects it as no longer addressable.
      const root = selectCurrentRootRun(agentSessionView().getCurrentView());
      // A steer needs the segment as well as the run: without it there is nothing to
      // address, and a fresh turn is the honest fallback.
      if (root?.status === "running" && sessionId && root.activeSegmentId) {
        if (
          steerRunningTurn({
            sessionId,
            runId: root.id,
            segmentId: root.activeSegmentId,
            input,
            send,
            runOptions,
          })
        ) {
          return true;
        }
      }
      return sendFreshTurn({ sessionId, send, input, runOptions });
    },
    [send],
  );
}

export function useCanSendToAgent(): boolean {
  const sessionId = useActiveSessionId();
  const send = agentSessionView().useAction("send");
  const root = useCurrentRootMaterial();
  return canAcceptChatInput(sessionId, Boolean(send), root.status);
}

export function canAcceptChatInput(
  sessionId: string,
  mountedSendAvailable: boolean,
  rootStatus: "idle" | "running" | "waiting" | "finished",
): boolean {
  // Only the mounted Session lifecycle may accept input. The projectless welcome
  // screen deliberately keeps the draft but cannot send it; a parked root must
  // be resumed through its interrupt rather than opened as a competing turn.
  return Boolean(sessionId) && mountedSendAvailable && rootStatus !== "waiting";
}

// A distinct "steer-" suffix so these cannot collide with send()'s own local-N counter.
// The fold reconciles them by CONTENT match, since runs.steer returns no item id.
const steerBubbleIds = new ExactSequence();

interface SteerRunningTurnInput {
  sessionId: string;
  runId: string;
  segmentId: string;
  input: AgentInput;
  send: SendToAgent | null;
  runOptions: AgentRunStartOptions;
}

function steerRunningTurn({
  sessionId,
  runId,
  segmentId,
  input,
  send,
  runOptions,
}: SteerRunningTurnInput): boolean {
  if (input.parts.length === 0) return false;
  const owner = agentCommandOwner();
  const runtime = agentRuntime();
  const view = agentSessionView();
  const localId = mintSteerBubble(view, sessionId, input);
  const effect = owner.trackEffect(() => view.dropMessage(sessionId, localId));
  void owner.settle(runtime.steerRun(runId, segmentId, input)).then(
    () => {
      if (owner.isCurrent()) effect.settle();
    },
    (err: unknown) => {
      if (!owner.isCurrent()) return;
      effect.rollback();
      // The addressed run finished, parked, or moved segment while the person typed. The
      // RUNTIME says which — not a guess here — and a fresh turn is what they meant.
      if (runtime.isRunGone(err)) {
        if (send?.(input, runOptions) !== true) {
          // Parked rather than finished: the input was not accepted as a new turn, so say
          // so instead of silently discarding an optimistic steer.
          notifyError(describeRpcError(err) ?? t("session.error.steer"), { source: "session" });
        }
        return;
      }
      // The steer may or may not have reached the loop, so the optimistic bubble goes
      // either way: if it landed the runtime streams the real Item back. Leaving it up is
      // the one outcome that lies — a message looking sent with no reply and no reason.
      console.error("[session] steer failed:", err);
      notifyError(describeRpcError(err) ?? t("session.error.steer"), { source: "session" });
    },
  );
  return true;
}

interface SendFreshTurnInput {
  sessionId: string;
  send: SendToAgent | null;
  input: AgentInput;
  runOptions: AgentRunStartOptions;
}

function sendFreshTurn({ sessionId, send, input, runOptions }: SendFreshTurnInput): boolean {
  if (sessionId && send) {
    return send(input, runOptions);
  }
  return false;
}

function mintSteerBubble(
  view: ReturnType<typeof agentSessionView>,
  sessionId: string,
  input: AgentInput,
): string {
  const id = `${OPTIMISTIC_STEER_MESSAGE_PREFIX}${steerBubbleIds.issue()}`;
  view.appendLocalUserMessage(sessionId, id, input);
  return id;
}
