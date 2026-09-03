import { useCallback, useRef, useState } from "react";
import type { InterruptResumePayload, ResolvePatch } from "../ports/sessionView";
import { agentSessionState } from "../ports/sessionState";
import { stageInterruptResponse } from "./interruptResponseCoordinator";

// The owning session is PINNED at mount: reading `activeSessionId` at click time lets a fast
// tab switch redirect the resume onto another session. Responses stage into the root Run's
// atomic interrupt set, and store-level settles are DEFERRED until the complete set opens a
// continuation (API.md §6).

/**
 * DEFERS every optimistic settle until the owning root's complete interrupt set has opened a
 * continuation, so a rejected resume (API.md §8.1) leaves the whole set intact and
 * retryable. False when this card is no longer part of an answerable set.
 */
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
  // Synchronous one-shot latch: `pending` only updates on the next render, so two submits in
  // one tick would both pass a state-based guard and fire two resumes. Cleared only on
  // channel-a failure — on success it stays latched, the interrupt being resolved.
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
      // No resume binding (session torn down) ⇒ never latched; roll back so the
      // card stays actionable. On success the latch stays (interrupt resolved).
      if (!resumeInterrupt(sessionId, runId, itemId, response, settled, { onError: rollback }))
        rollback();
    },
    [runId, itemId, sessionId],
  );

  return { pending, resume, sessionId };
}
