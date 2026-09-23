import { useMemo } from "react";
import type {
  AgentProblem,
  AgentRunMetrics,
  AgentModelSelection,
  AgentRunOutcome,
  AgentRunView,
  TimelineEntry,
  ToolCall,
} from "@/plugins/sdk/types/agentSessionView";
import { agentSessionView } from "../ports/sessionView";
import type { AgentRootAttention, AgentRunTreeNode } from "../view/runTree";
import type { TranscriptRow } from "../conversation/transcriptRows";
import { isAgentRunFailure } from "../view/runOutcome";

export class CurrentRootMaterial {
  static readonly idle = new CurrentRootMaterial(null);

  readonly runId: string | null;
  readonly status: AgentRunView["status"] | "idle";
  readonly outcome: AgentRunOutcome | null;
  readonly metrics: AgentRunMetrics | null;
  readonly contextTokens: number | null;
  readonly modelSelection: AgentModelSelection | null;
  readonly startedAt: number | null;
  readonly attention: AgentRootAttention;

  private constructor(run: AgentRunView | null) {
    this.runId = run?.id ?? null;
    this.status = run?.status ?? "idle";
    this.outcome = run?.outcome ?? null;
    this.metrics = run?.metrics ?? null;
    this.contextTokens = run?.contextTokens ?? null;
    this.modelSelection = run?.modelSelection ?? null;
    this.startedAt = epochMillis(run?.createdAt);
    this.attention = Object.freeze(
      run ? { status: run.status, runId: run.id } : { status: "idle", runId: null },
    );
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
      isAgentRunFailure(this.outcome)
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
  const run = agentSessionView().useCurrentRootRun();
  return useMemo(() => CurrentRootMaterial.from(run), [run]);
}

export function useIsCurrentRootRunning(): boolean {
  return agentSessionView().useCurrentRootRunning();
}

export function useActiveSessionToolCalls(): Record<string, ToolCall> {
  return agentSessionView().useToolCalls();
}

export function useActiveSessionTimeline(): TimelineEntry[] {
  return agentSessionView().useSessionTimeline();
}

export function useActiveSessionRunTree(): AgentRunTreeNode[] {
  return agentSessionView().useRunTree();
}

export function useActiveSessionProblem(): AgentProblem | null {
  return agentSessionView().useProblem();
}

function epochMillis(iso: string | undefined): number | null {
  if (iso === undefined) return null;
  const parsed = Date.parse(iso);
  return Number.isNaN(parsed) ? null : parsed;
}
