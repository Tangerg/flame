import { useAgentDependencies } from "@/features/agent/application/agentContext";
import { useMemo } from "react";
import {
  type AgentProblem,
  type AgentModelSelection,
  type AgentRunOutcome,
  type AgentRunView,
} from "@/features/agent/public/sessionView";

import { type TranscriptRow } from "@/features/agent/application/conversation/transcriptRows";
import { isAgentRunFailure } from "@/features/agent/application/view/runOutcome";
export class CurrentRootMaterial {
  static readonly idle = new CurrentRootMaterial(null);

  readonly runId: string | null;
  readonly status: AgentRunView["status"] | "idle";
  readonly outcome: AgentRunOutcome | null;
  readonly contextTokens: number | null;
  readonly modelSelection: AgentModelSelection | null;
  readonly startedAt: number | null;

  private constructor(run: AgentRunView | null) {
    this.runId = run?.id ?? null;
    this.status = run?.status ?? "idle";
    this.outcome = run?.outcome ?? null;
    this.contextTokens = run?.contextTokens ?? null;
    this.modelSelection = run?.modelSelection ?? null;
    this.startedAt = epochMillis(run?.createdAt);
    Object.freeze(this);
  }

  static from(run: AgentRunView | null): CurrentRootMaterial {
    return run ? new CurrentRootMaterial(run) : CurrentRootMaterial.idle;
  }

  get running(): boolean {
    return this.status === "running";
  }

  terminalTurnIndex(rows: readonly TranscriptRow[]): number {
    if (
      this.status !== "finished" ||
      this.runId === null ||
      this.outcome === null ||
      (isAgentRunFailure(this.outcome) && !this.outcome.unresolvedEffects?.length)
    ) {
      return -1;
    }
    for (let index = rows.length - 1; index >= 0; index -= 1) {
      const owner = rows[index]!.runOwner;
      if (owner.kind === "owned" && owner.runId === this.runId) return index;
    }
    return -1;
  }
}

export function useCurrentRootMaterial(): CurrentRootMaterial {
  const agentAccess = useAgentDependencies();

  const run = agentAccess.view.useCurrentRootRun();
  return useMemo(() => CurrentRootMaterial.from(run), [run]);
}

export function useIsCurrentRootRunning(): boolean {
  const agentAccess = useAgentDependencies();

  return agentAccess.view.useCurrentRootRunning();
}

export function useActiveSessionProblem(): AgentProblem | null {
  const agentAccess = useAgentDependencies();

  return agentAccess.view.useProblem();
}

function epochMillis(iso: string | undefined): number | null {
  if (iso === undefined) return null;
  const parsed = Date.parse(iso);
  return Number.isNaN(parsed) ? null : parsed;
}
