import { useCallback, useRef, useState } from "react";
import type { InterruptResumePayload, ResolvePatch } from "../ports/sessionView";
import { agentSessionState } from "../ports/sessionState";
import { stageInterruptResponse } from "./interruptResponseCoordinator";

export function resumeInterrupt(
  sessionId: string,
  runId: string,
  itemId: string,
  response: InterruptResumePayload,
  settled: ResolvePatch,
  hooks?: { onSettled?: () => void; onError?: () => void },
): boolean {
  return stageInterruptResponse({ sessionId, rootRunId: runId, itemId }, response, settled, hooks);
}

export function useInterruptResume<P>(runId?: string, itemId?: string) {
  const [pending, setPending] = useState<P | null>(null);
  const [sessionId] = useState(() => agentSessionState().getActiveSessionId());
  const submitted = useRef(false);

  const resume = useCallback(
    (marker: P, response: InterruptResumePayload, settled: ResolvePatch) => {
      if (!runId || !itemId || submitted.current) return;
      submitted.current = true;
      setPending(marker);
      const rollback = () => {
        submitted.current = false;
        setPending(null);
      };
      if (
        !resumeInterrupt(sessionId, runId, itemId, response, settled, {
          onSettled: () => setPending(null),
          onError: rollback,
        })
      )
        rollback();
    },
    [runId, itemId, sessionId],
  );

  return { pending, resume, sessionId };
}
