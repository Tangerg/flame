import { type AgentAccess, useAgent } from "@/features/agent/public/agent";
import { useCallback, useState } from "react";
import { type InterruptResumePayload } from "@/features/agent/domain/hitl";

export function resumeInterrupt(
  agentAccess: AgentAccess,
  sessionId: string,
  runId: string,
  itemId: string,
  response: InterruptResumePayload,
  hooks?: { onSettled?: () => void; onError?: () => void },
): boolean {
  return agentAccess.interrupts.stage({ sessionId, rootRunId: runId, itemId }, response, hooks);
}

export function useInterruptResume<P>(sessionId: string, runId?: string, itemId?: string) {
  const agentAccess = useAgent();

  const [pending, setPending] = useState<P | null>(null);

  const resume = useCallback(
    (marker: P, response: InterruptResumePayload) => {
      if (!runId || !itemId || pending !== null) return;
      setPending(marker);
      const clear = () => setPending(null);
      if (
        !resumeInterrupt(agentAccess, sessionId, runId, itemId, response, {
          onSettled: clear,
          onError: clear,
        })
      )
        clear();
    },
    [sessionId, runId, itemId, pending, agentAccess],
  );

  // The coordinator is the sole owner of whether this interrupt already carries a
  // response in flight or accepted; the card observes that fact instead of latching
  // its own copy, so a remount can never resurrect a settled submission.
  const submitted =
    runId !== undefined && itemId !== undefined
      ? agentAccess.interrupts.isStaged({ sessionId, rootRunId: runId, itemId })
      : false;

  return { pending, submitted, resume };
}
