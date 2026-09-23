import type { AgentSessionView } from "@/plugins/sdk/types/agentSessionView";
import type { AgentEventEnvelope } from "./agentEvents";

export type StreamEventHandler = (
  state: AgentSessionView,
  event: AgentEventEnvelope,
) => AgentSessionView;
