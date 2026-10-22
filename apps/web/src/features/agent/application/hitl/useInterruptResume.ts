import { useCallback, useRef, useState } from "react";
import { type InterruptResumePayload } from "@/features/agent/domain/hitl";
import { agentSessionState } from "@/features/agent/application/ports/sessionState";
import { stageInterruptResponse } from "@/features/agent/application/hitl/interruptResponseCoordinator";
export function resumeInterrupt(
  sessionId: string,
  runId: string,
  itemId: string,
  response: InterruptResumePayload,
  hooks?: { onSettled?: () => void; onError?: () => void },
): boolean {
  return stageInterruptResponse({ sessionId, rootRunId: runId, itemId }, response, hooks);
}

export function useInterruptResume<P>(runId?: string, itemId?: string) {
  const [pending, setPending] = useState<P | null>(null);
  // `pending` clears on settle so a spinner stops; `done` stays so the controls
  // remain disabled until the card unmounts, matching the submit latch.
  const [done, setDone] = useState(false);
  const [sessionId] = useState(() => agentSessionState().getActiveSessionId());
  const submitted = useRef(false);

  const resume = useCallback(
    (marker: P, response: InterruptResumePayload) => {
      if (!runId || !itemId || submitted.current) return;
      submitted.current = true;
      setDone(true);
      setPending(marker);
      const rollback = () => {
        submitted.current = false;
        setDone(false);
        setPending(null);
      };
      if (
        !resumeInterrupt(sessionId, runId, itemId, response, {
          onSettled: () => setPending(null),
          onError: rollback,
        })
      )
        rollback();
    },
    [runId, itemId, sessionId],
  );

  return { pending, done, resume, sessionId };
}
