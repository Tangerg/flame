import type { Message } from "@/plugins/sdk/types/agentSessionView";

export type SteerInputFeedback =
  "pending" | "unknown" | "accepted" | "applied" | "notApplied" | "retired";

export function steerInputFeedback(message: Message): SteerInputFeedback | undefined {
  if (!message.steer) return undefined;
  if (message.steer.receipt === "retired") return "retired";
  if (message.runId === message.steer.runId) return "applied";
  return message.steer.application ?? message.steer.receipt ?? message.steer.delivery;
}
