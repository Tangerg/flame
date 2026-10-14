import type { AgentSessionView, Message } from "@/plugins/sdk/types/agentSessionView";

function assertSteerOwner(message: Message, runId: string): void {
  if (
    message.role !== "user" ||
    (message.runId !== null && message.runId !== runId) ||
    (message.steer && message.steer.runId !== runId)
  )
    throw new Error("agent.steer.itemOwnerMismatch");
}

export function projectSteerReceipt(
  view: AgentSessionView,
  messageId: string,
  steer: Required<Pick<NonNullable<Message["steer"]>, "runId" | "receipt">>,
): AgentSessionView {
  const message = view.messages.find((current) => current.id === messageId);
  if (!message) return view;
  assertSteerOwner(message, steer.runId);
  return {
    ...view,
    messages: view.messages.map((current) =>
      current === message
        ? { ...current, steer: { ...current.steer, ...steer, delivery: undefined } }
        : current,
    ),
  };
}

export function projectSteerDelivery(
  view: AgentSessionView,
  messageId: string,
  command: Pick<NonNullable<Message["steer"]>, "runId" | "delivery">,
): AgentSessionView {
  const message = view.messages.find((current) => current.id === messageId);
  if (!message) return view;
  assertSteerOwner(message, command.runId);
  return {
    ...view,
    messages: view.messages.map((current) =>
      current === message ? { ...current, steer: { ...current.steer, ...command } } : current,
    ),
  };
}

export function reconcileMessageIdentity(
  view: AgentSessionView,
  fromId: string,
  toId: string,
  steer?: Required<Pick<NonNullable<Message["steer"]>, "runId" | "receipt">>,
): AgentSessionView {
  if (fromId === toId) return steer ? projectSteerReceipt(view, toId, steer) : view;
  const has = (id: string) => view.messages.some((message) => message.id === id);
  if (!has(fromId) && !(steer && has(toId))) return view;
  const targetExists = has(toId);
  const source = view.messages.find((message) => message.id === fromId);
  const target = view.messages.find((message) => message.id === toId);
  const association = steer ?? source?.steer;
  if (association && source) assertSteerOwner(source, association.runId);
  if (association && target) assertSteerOwner(target, association.runId);
  return {
    ...view,
    messages: targetExists
      ? view.messages
          .filter((message) => message.id !== fromId)
          .map((message) =>
            message.id === toId && association
              ? {
                  ...message,
                  steer: {
                    ...association,
                    ...message.steer,
                    ...(steer ? { ...steer, delivery: undefined } : {}),
                  },
                }
              : message,
          )
      : view.messages.map((message) =>
          message.id === fromId
            ? {
                ...message,
                id: toId,
                ...(steer ? { steer: { ...message.steer, ...steer, delivery: undefined } } : {}),
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
): AgentSessionView {
  const pending = previous.messages.filter(
    (message) =>
      message.steer?.delivery &&
      !message.steer.receipt &&
      !authoritative.messages.some((current) => current.id === message.id),
  );
  return pending.length
    ? { ...authoritative, messages: [...authoritative.messages, ...pending] }
    : authoritative;
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
