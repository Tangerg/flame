import type { AgentSessionView, Message } from "@/plugins/sdk/types/agentSessionView";

export function projectSteerReceipt(
  view: AgentSessionView,
  messageId: string,
  steer: NonNullable<Message["steer"]>,
): AgentSessionView {
  if (!view.messages.some((message) => message.id === messageId)) return view;
  return {
    ...view,
    messages: view.messages.map((message) =>
      message.id === messageId ? { ...message, steer } : message,
    ),
  };
}

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
              ? { ...message, steer: { runId: steerRunId, receipt: "accepted" } }
              : message,
          )
      : view.messages.map((message) =>
          message.id === fromId
            ? {
                ...message,
                id: toId,
                ...(steerRunId
                  ? { steer: { runId: steerRunId, receipt: "accepted" as const } }
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
    const index = messages.findIndex((message) => message.id === pending.id);
    if (index >= 0) {
      if (messages[index]!.runId === steer.runId)
        messages[index] = { ...messages[index]!, steer: { ...steer, receipt: "accepted" } };
    } else if (pending.runId !== steer.runId) {
      const run = authoritative.runsById[steer.runId];
      if (steer.receipt !== "accepted") messages.push(pending);
      else if (run?.status === "finished") unapplied = true;
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
