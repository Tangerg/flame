import type { SessionSnapshot } from "@flame/runtime-contract/client";
import type { AgentSessionMaterialRead } from "../application/ports/runtimeGateway";
import { stageAgentSessionSharedMaterial } from "../application/ports/sessionSharedMaterial";
import { runtimePlan } from "./runtimePlan";
import { runtimeItem, runtimePendingInterruptSet, runtimeRunFact } from "./runtimeAgentFacts";

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
      ...(plan ? { plan } : {}),
    },
    projectAssociatedSharedMaterial: stageAgentSessionSharedMaterial(sessionId, snapshot),
  };
}
