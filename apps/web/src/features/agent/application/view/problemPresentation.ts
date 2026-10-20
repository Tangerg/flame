import { type AgentProblem, type AgentSessionView } from "@/features/agent/public/sessionView";
import { isAgentRunFailure } from "@/features/agent/application/view/runOutcome";
import { selectCurrentRootRun, selectRunProblem } from "@/features/agent/application/view/runTree";
export interface ProblemPresentation {
  readonly commandError: AgentProblem | null;
  readonly dismissedRunId: string | null;
}

export const EMPTY_PROBLEM_PRESENTATION: ProblemPresentation = {
  commandError: null,
  dismissedRunId: null,
};

export function selectVisibleProblem(
  view: AgentSessionView,
  presentation: ProblemPresentation,
): AgentProblem | null {
  if (presentation.commandError) return presentation.commandError;
  const run = selectCurrentRootRun(view);
  return run?.id === presentation.dismissedRunId ? null : selectRunProblem(run);
}

export function withCommandError(
  presentation: ProblemPresentation,
  error: AgentProblem | null,
): ProblemPresentation {
  if (presentation.commandError === error) return presentation;
  return { ...presentation, commandError: error };
}

export function dismissVisibleProblem(
  view: AgentSessionView,
  presentation: ProblemPresentation,
): ProblemPresentation {
  const run = selectCurrentRootRun(view);
  const dismissedRunId = isAgentRunFailure(run?.outcome) ? run.id : presentation.dismissedRunId;
  if (presentation.commandError === null && dismissedRunId === presentation.dismissedRunId) {
    return presentation;
  }
  return { commandError: null, dismissedRunId };
}
