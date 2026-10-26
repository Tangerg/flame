import {
  type AgentSessionStatePort,
  type AgentSessionViewPort,
  type AgentDefaultSessionPort,
} from "@/features/agent/application/session/sessionAccess";
import { type AgentRuntimeGateway } from "@/features/agent/application/runtimeGateway";
import { type AgentCommands } from "@/features/agent/application/agentCommandOwner";
import { type createAgentSessionUsage } from "@/features/agent/application/session/sessionUsage";
import { type AgentSharedMaterial } from "@/features/agent/application/session/sharedMaterial";
import { type InterruptResponseCoordinator } from "@/features/agent/application/hitl/interruptResponseCoordinator";
export interface AgentAccess {
  readonly state: AgentSessionStatePort;
  readonly view: AgentSessionViewPort;
  readonly defaultSession: AgentDefaultSessionPort;
  readonly runtime: AgentRuntimeGateway;
  readonly commands: AgentCommands;
  readonly usage: ReturnType<typeof createAgentSessionUsage>;
  readonly sharedMaterial: AgentSharedMaterial;
  readonly interrupts: InterruptResponseCoordinator;
}
