import type { SessionSnapshot } from "@flame/runtime-contract/client";
import type { AgentSessionMaterialRead } from "../application/ports/runtimeGateway";
import { stageAgentSessionSharedMaterial } from "../application/ports/sessionSharedMaterial";
import { runtimePlan } from "./runtimePlan";
import { runtimeItem, runtimePendingInterruptSet, runtimeRunFact } from "./runtimeAgentFacts";
import { contentBlocksToAgentInput } from "./wireInput";

export function runtimeSessionMaterial(
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
    projectAssociatedSharedMaterial: stageAgentSessionSharedMaterial(sessionId, snapshot),
  };
}
