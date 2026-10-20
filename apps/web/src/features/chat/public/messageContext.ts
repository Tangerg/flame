import { type Message } from "@/features/agent/public/sessionView";
import { createContext, use } from "react";
export interface MessageContextValue {
  sessionId: string;
  message: Message;
}

export const MessageContext = createContext<MessageContextValue | null>(null);

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
