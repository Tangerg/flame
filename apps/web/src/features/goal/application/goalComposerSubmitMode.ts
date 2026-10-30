import { type AgentRunStartOptions } from "@/features/agent/public/runOptions";
import {
  type ComposerSubmitModeContext,
  type ComposerSubmitModeSpec,
} from "@/features/composer/public/types";
import { type GoalReadModel, type GoalState } from "@/features/goal/application/goalReadModel";
import { goalCanResume } from "@/features/goal/application/goalStatusPresentation";
import { type StartGoalInput } from "@/features/goal/application/ports/goalCommandsGateway";
import {
  GoalComposerModeOwner,
  type GoalComposerSubmission,
} from "@/features/goal/application/goalComposerMode";
export interface GoalComposerSubmitDependencies {
  getActiveSessionId(): string | null;
  goalState(sessionId: string): GoalState | undefined;
  runtimeAvailable(): boolean;
  runOptions(): AgentRunStartOptions;
  captureStart(): (input: StartGoalInput) => Promise<void>;
  focusComposer(): void;
  reportUnavailable(): void;
  reportUnsupportedAttachments(): void;
  reportStartError(error: unknown): void;
  retired(error: unknown): boolean;
}

export function createGoalComposerSubmitMode(
  owner: GoalComposerModeOwner,
  dependencies: GoalComposerSubmitDependencies,
): ComposerSubmitModeSpec {
  return {
    id: "goal",
    matches: (draft) => {
      const sessionId = dependencies.getActiveSessionId();
      return draft.slash?.command === "/goal" || Boolean(sessionId && owner.active(sessionId));
    },
    submit: (context) => submitGoalComposerMode(owner, dependencies, context),
  };
}

function submitGoalComposerMode(
  owner: GoalComposerModeOwner,
  dependencies: GoalComposerSubmitDependencies,
  context: ComposerSubmitModeContext,
): void {
  const sessionId = dependencies.getActiveSessionId();
  if (!sessionId || owner.retired || !dependencies.runtimeAvailable()) {
    dependencies.reportUnavailable();
    return;
  }

  const state = dependencies.goalState(sessionId);
  if (state?.available !== true) {
    dependencies.reportUnavailable();
    return;
  }

  if (context.hasImages || context.hasPastes) {
    dependencies.reportUnsupportedAttachments();
    return;
  }

  const goalCommand = context.slash?.command === "/goal";
  const objective = (goalCommand ? context.slash?.args : context.text)?.trim() ?? "";
  const existing = state.goal;
  if (!goalCanEnterComposerMode(existing)) {
    dependencies.reportUnavailable();
    return;
  }

  if (goalCommand && !owner.active(sessionId) && !owner.activate(sessionId)) return;

  if (!objective) {
    if (goalCommand) {
      context.clear();
      dependencies.focusComposer();
    }
    return;
  }

  const startCommand = dependencies.captureStart();
  const start = (submission: GoalComposerSubmission) => {
    void startGoal(submission, dependencies, startCommand, context, sessionId, objective);
  };
  if (existing) {
    owner.requestReplacement(sessionId, existing.objective, start);
    return;
  }
  const submission = owner.begin(sessionId);
  if (submission) start(submission);
}

async function startGoal(
  submission: GoalComposerSubmission,
  dependencies: GoalComposerSubmitDependencies,
  startCommand: (input: StartGoalInput) => Promise<void>,
  context: ComposerSubmitModeContext,
  sessionId: string,
  objective: string,
): Promise<void> {
  try {
    await startCommand({ sessionId, objective, ...dependencies.runOptions() });
    const committed = submission.finish(true);
    if (committed) context.accept();
  } catch (error) {
    submission.finish(false);
    if (!dependencies.retired(error)) dependencies.reportStartError(error);
  }
}

export function goalCanEnterComposerMode(goal: GoalReadModel | null): boolean {
  return !goal || goalCanResume(goal);
}
