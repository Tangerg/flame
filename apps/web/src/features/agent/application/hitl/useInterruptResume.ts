import { useCallback, useSyncExternalStore } from "react";
import { useAgentDependencies } from "@/features/agent/application/agentContext";
import { type InterruptResumePayload } from "@/features/agent/domain/hitl";

export function useInterruptResume(sessionId: string, rootRunId?: string, itemId?: string) {
  const { interrupts } = useAgentDependencies();
  const state = useSyncExternalStore(interrupts.subscribe, () =>
    itemId === undefined ? "unsubmitted" : interrupts.responseState(sessionId, itemId),
  );
  const submit = useCallback(
    (response: InterruptResumePayload) => {
      if (rootRunId === undefined || itemId === undefined) return;
      interrupts.stage({ sessionId, rootRunId, itemId }, response);
    },
    [interrupts, sessionId, rootRunId, itemId],
  );
  const available = rootRunId !== undefined && itemId !== undefined && state === "unsubmitted";
  return { state, available, submit };
}
