import { type Message } from "@/features/agent/public/sessionView";
import { type HistoryEditScope } from "@/features/chat/message-actions/public/messageActions";
import { createContext, use } from "react";

export interface MessageContextValue {
  sessionId: string;
  message: Message;
  editInComposer(origin?: HTMLElement): void;
  confirmHistoryEdit(scope: HistoryEditScope, apply: () => boolean): void;
}

export const MessageContext = createContext<MessageContextValue | null>(null);

export function useMessageContext(): MessageContextValue {
  const ctx = use(MessageContext);
  if (!ctx) throw new Error("useMessageContext() must be called inside a MessageBlock");
  return ctx;
}

export function useCurrentMessage(): Message {
  return useMessageContext().message;
}

export function useCurrentMessageSessionId(): string {
  return useMessageContext().sessionId;
}
