import {
  type AgentProblem,
  type AgentRunFailureOutcome,
  type AgentSessionView,
} from "@/features/agent/public/sessionView";
import { isAgentRunFailure } from "@/features/agent/application/view/runOutcome";
import { selectCurrentRootRun } from "@/features/agent/application/view/runTree";

export type DisplayedSessionProblem = { readonly sessionId: string } & (
  | { readonly kind: "command"; readonly error: AgentProblem }
  | {
      readonly kind: "run";
      readonly runId: string;
      readonly outcome: AgentRunFailureOutcome;
    }
);

export class ProblemPresentation {
  static readonly empty = new ProblemPresentation(null, null);

  private constructor(
    private readonly commandError: AgentProblem | null,
    private readonly dismissedRunId: string | null,
  ) {}

  withCommandError(error: AgentProblem | null): ProblemPresentation {
    return error === this.commandError ? this : new ProblemPresentation(error, this.dismissedRunId);
  }

  displayed(sessionId: string, view: AgentSessionView): DisplayedSessionProblem | null {
    if (this.commandError) return { kind: "command", sessionId, error: this.commandError };
    const run = selectCurrentRootRun(view);
    if (!run || run.id === this.dismissedRunId || !isAgentRunFailure(run.outcome)) return null;
    return { kind: "run", sessionId, runId: run.id, outcome: run.outcome };
  }

  dismiss(view: AgentSessionView, target: DisplayedSessionProblem): ProblemPresentation {
    const current = this.displayed(target.sessionId, view);
    if (target.kind === "command" && current?.kind === "command" && current.error === target.error)
      return this.withCommandError(null);
    if (target.kind === "run" && current?.kind === "run" && current.runId === target.runId)
      return new ProblemPresentation(this.commandError, target.runId);
    return this;
  }
}
