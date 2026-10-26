import { type SessionSnapshot } from "@flame/runtime-client";
import { type AgentSessionMaterialRead } from "@/features/agent/application/runtimeGateway";
import { type AgentSharedMaterial } from "@/features/agent/application/session/sharedMaterial";
import { runtimePlan } from "@/features/agent/adapters/runtimePlan";
import {
  runtimeItem,
  runtimePendingInterruptSet,
  runtimeRunFact,
} from "@/features/agent/adapters/runtimeAgentFacts";
import { contentBlocksToAgentInput } from "@/features/agent/adapters/wireInput";
export function runtimeSessionMaterial(
  sharedMaterial: AgentSharedMaterial,
  sessionId: string,
  snapshot: SessionSnapshot,
): AgentSessionMaterialRead {
  const plan = snapshot.plan ? runtimePlan(snapshot.plan) : undefined;
  return {
    snapshot: {
      items: snapshot.items.map(runtimeItem),
      runs: snapshot.runs.map(runtimeRunFact),
      pendingInterruptSets: snapshot.interrupts.map(runtimePendingInterruptSet),
      steerInputs: snapshot.steerInputs?.map((steer) => ({
        runId: steer.runId,
        userItemId: steer.userItemId,
        input: contentBlocksToAgentInput(steer.input),
        receipt: "accepted",
      })),
      ...(plan ? { plan } : {}),
    },
    projectAssociatedSharedMaterial: sharedMaterial.stage(sessionId, snapshot),
  };
}
