import type { AgentSessionView } from "@/plugins/sdk/types/agentSessionView";

export function reconcileMessageIdentity(
  view: AgentSessionView,
  fromId: string,
  toId: string,
  steerRunId?: string,
): AgentSessionView {
  if (fromId === toId) return view;
  const has = (id: string) => view.messages.some((message) => message.id === id);
  if (!has(fromId) && !(steerRunId && has(toId))) return view;
  const targetExists = has(toId);
  return {
    ...view,
    messages: targetExists
      ? view.messages
          .filter((message) => message.id !== fromId)
          .map((message) =>
            message.id === toId && steerRunId
              ? {
                  ...message,
                  steer: {
                    runId: steerRunId,
                    status:
                      message.runId === steerRunId ? ("applied" as const) : ("accepted" as const),
                  },
                }
              : message,
          )
      : view.messages.map((message) =>
          message.id === fromId
            ? {
                ...message,
                id: toId,
                ...(steerRunId
                  ? { steer: { runId: steerRunId, status: "accepted" as const } }
                  : {}),
              }
            : message,
        ),
    assistantTurnByRunId: Object.fromEntries(
      Object.entries(view.assistantTurnByRunId).map(([runId, messageId]) => [
        runId,
        messageId === fromId ? toId : messageId,
      ]),
    ),
  };
}

export function reconcileSteerMessages(
  previous: AgentSessionView,
  authoritative: AgentSessionView,
): { view: AgentSessionView; unapplied: boolean } {
  const receipts = previous.messages.filter((message) => message.steer);
  if (receipts.length === 0) return { view: authoritative, unapplied: false };
  const messages = [...authoritative.messages];
  let unapplied = false;
  for (const pending of receipts) {
    const steer = pending.steer!;
    const index = messages.findIndex(
      (message) => message.id === pending.id && message.runId === steer.runId,
    );
    if (index >= 0) {
      messages[index] = { ...messages[index]!, steer: { ...steer, status: "applied" } };
    } else if (steer.status === "accepted") {
      const run = authoritative.runsById[steer.runId];
      if (run?.status === "finished") unapplied = true;
      else if (run) messages.push(pending);
    }
  }
  return { view: { ...authoritative, messages }, unapplied };
}

export function dropMessage(view: AgentSessionView, id: string): AgentSessionView {
  if (!view.messages.some((message) => message.id === id)) return view;
  return {
    ...view,
    messages: view.messages.filter((message) => message.id !== id),
    assistantTurnByRunId: Object.fromEntries(
      Object.entries(view.assistantTurnByRunId).filter(([, messageId]) => messageId !== id),
    ),
  };
}
