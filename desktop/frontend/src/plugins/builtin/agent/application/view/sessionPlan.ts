import { useMemo } from "react";
import type { AgentPlan, PlanStep } from "@/plugins/sdk/types/agentSessionView";
import { agentSessionView } from "../ports/sessionView";
import { useActiveSessionId } from "../session/activeSession";
import { tupleKey } from "@/lib/tupleKey";

export type { PlanStep } from "@/plugins/sdk/types/agentSessionView";

const NO_STEPS: readonly PlanStep[] = Object.freeze([]);

export function planSteps(plan: AgentPlan | undefined): readonly PlanStep[] {
  const steps = plan?.steps;
  if (!steps || steps.length === 0) return NO_STEPS;
  return Object.freeze(steps.map((step) => Object.freeze({ ...step })));
}

export class SessionPlan {
  readonly identity: string;
  readonly steps: readonly PlanStep[];

  private constructor(sessionId: string, generation: bigint, steps: readonly PlanStep[]) {
    this.identity = tupleKey(sessionId, generation.toString());
    this.steps = steps;
  }

  static fromSnapshot(
    sessionId: string,
    generation: bigint,
    plan: AgentPlan | undefined,
  ): SessionPlan {
    return new SessionPlan(sessionId, generation, planSteps(plan));
  }

  activeStep(): PlanStep | undefined {
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

export function useSessionPlan(): SessionPlan {
  const sessionId = useActiveSessionId();
  const material = agentSessionView().usePlan();
  return useMemo(
    () => SessionPlan.fromSnapshot(sessionId, material.generation, material.value),
    [material, sessionId],
  );
}
