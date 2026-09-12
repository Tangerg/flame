import { useMemo } from "react";
import type { AgentPlan, PlanStep } from "@/plugins/sdk/types/agentSessionView";
import { agentSessionView } from "../ports/sessionView";
import { useActiveSessionId } from "../session/activeSession";
import { tupleKey } from "@/lib/tupleKey";

export type { PlanStep } from "@/plugins/sdk/types/agentSessionView";

// The Plan is a SESSION projection written only by the root Run, not a transcript Item —
// it has no run of its own and nothing about it is per-turn.
const NO_STEPS: readonly PlanStep[] = Object.freeze([]);

export function planSteps(plan: AgentPlan | undefined): readonly PlanStep[] {
  const steps = plan?.steps;
  if (!steps || steps.length === 0) return NO_STEPS;
  return Object.freeze(steps.map((step) => Object.freeze({ ...step })));
}

// The Runtime revision identifies a whole replacement WITHIN one projection generation;
// `generation` is what stops a server/recovery successor carrying the same session and
// revision from inheriting its predecessor's presentation state.
export class SessionPlan {
  readonly identity: string;
  readonly generation: bigint;
  readonly revision: number | undefined;
  readonly steps: readonly PlanStep[];

  private constructor(
    sessionId: string,
    generation: bigint,
    revision: number | undefined,
    steps: readonly PlanStep[],
  ) {
    this.identity = tupleKey(
      sessionId,
      generation.toString(),
      revision === undefined ? "unwritten" : "committed",
      ...(revision === undefined ? [] : [String(revision)]),
    );
    this.generation = generation;
    this.revision = revision;
    this.steps = steps;
  }

  static fromSnapshot(
    sessionId: string,
    generation: bigint,
    plan: AgentPlan | undefined,
  ): SessionPlan {
    return new SessionPlan(sessionId, generation, plan?.revision, planSteps(plan));
  }

  activeStep(): PlanStep | undefined {
    // An active step outranks an earlier untouched step.
    return (
      this.steps.find((step) => step.status === "active") ??
      this.steps.find((step) => step.status === "pending")
    );
  }

  progress(): { done: number; total: number } {
    return {
      done: this.steps.filter((step) => step.status === "done").length,
      total: this.steps.length,
    };
  }
}

// Memoised on session identity and the snapshot object the fold swaps in, so a reader keeps
// one stable model across unrelated renders.
export function useSessionPlan(): SessionPlan {
  const sessionId = useActiveSessionId();
  const material = agentSessionView().usePlan();
  return useMemo(
    () => SessionPlan.fromSnapshot(sessionId, material.generation, material.value),
    [material, sessionId],
  );
}
