import type { AgentSessionView } from "@/plugins/sdk/types/agentSessionView";
import type { AgentEventEnvelope } from "./agentEvents";

/**
 * Core Runtime facts are folded before optional plugin projections.
 * Multiple plugins may register for one type; they run in REGISTRATION ORDER, each seeing
 * the previous output. The envelope is mandatory provenance — source Run, Segment, event
 * identity and runtime timestamp cannot be reconstructed from a payload or from UI state.
 */
export type StreamEventHandler = (
  state: AgentSessionView,
  event: AgentEventEnvelope,
) => AgentSessionView;
