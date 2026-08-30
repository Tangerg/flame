// Its own module so an SDK consumer can import the hook without dragging in `MessageBlock`'s
// React tree.

import type { Message } from "@/plugins/sdk/types/agentSessionView";
import { createContext, use } from "react";

export interface MessageContextValue {
  sessionId: string;
  message: Message;
}

export const MessageContext = createContext<MessageContextValue | null>(null);

/** Throws outside a MessageBlock, which is almost certainly a plugin-author bug. */
export function useCurrentMessage(): Message {
  const ctx = use(MessageContext);
  if (!ctx) throw new Error("useCurrentMessage() must be called inside a MessageBlock");
  return ctx.message;
}

export function useCurrentMessageSessionId(): string {
  const ctx = use(MessageContext);
  if (!ctx) throw new Error("useCurrentMessageSessionId() must be called inside a MessageBlock");
  return ctx.sessionId;
}
