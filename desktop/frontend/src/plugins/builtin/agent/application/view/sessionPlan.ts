import { useMemo } from "react";
import type { AgentPlan, PlanStep } from "@/plugins/sdk/types/agentSessionView";
import { agentSessionView } from "../ports/sessionView";
import { useActiveSessionId } from "../session/activeSession";
import { tupleKey } from "@/lib/tupleKey";

export type { PlanStep } from "@/plugins/sdk/types/agentSessionView";

// The Plan is a SESSION projection written only by the root Run, not a transcript Item —
// it has no run of its own and nothing about it is per-turn.
const TOOL_STEP_STATUS = new Map<string, PlanStep["status"]>([
  ["completed", "done"],
  ["in_progress", "active"],
  ["pending", "pending"],
]);

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
    return activePlanStep(this.steps);
  }

  progress(): { done: number; total: number } {
    return planProgress(this.steps);
  }
}

/**
 * What one `set_plan` call SAID, as opposed to what the session's plan IS.
 *
 * Reads the structured arguments, NOT the rendered `[x] …` result text the runtime also
 * produces for the model: parsing that back would be a second answer to "what are the
 * steps" that goes stale the moment the marks change. Arguments carry no ids, so the index
 * stands in — which is all a list key needs.
 */
export function planStepsFromArguments(args: unknown): readonly PlanStep[] {
  if (typeof args !== "object" || args === null) return NO_STEPS;
  const steps = (args as { steps?: unknown }).steps;
  if (!Array.isArray(steps) || steps.length === 0) return NO_STEPS;
  const projected: PlanStep[] = [];
  for (const [index, step] of steps.entries()) {
    if (typeof step !== "object" || step === null) continue;
    const { description, status } = step as { description?: unknown; status?: unknown };
    if (typeof description !== "string" || description.length === 0) continue;
    projected.push({
      id: String(index),
      text: description,
      status: TOOL_STEP_STATUS.get(String(status)) ?? "pending",
    });
  }
  return projected;
}

// The MARK outranks position: "first one not done" agrees on the common plan but names the
// wrong step when an active step sits after an untouched one — exactly the plan where a
// reader most needs to be told.
export function activePlanStep(steps: readonly PlanStep[]): PlanStep | undefined {
  return (
    steps.find((step) => step.status === "active") ??
    steps.find((step) => step.status === "pending")
  );
}

export function planProgress(steps: readonly PlanStep[]): { done: number; total: number } {
  return { done: steps.filter((step) => step.status === "done").length, total: steps.length };
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
