import { type AgentRunStartOptions } from "@/features/agent/public/runOptions";
import {
  type ComposerSubmitModeContext,
  type ComposerSubmitModeSpec,
} from "@/features/composer/public/types";
import {
  GOAL_SLASH_COMMAND,
  type GoalReadModel,
  type GoalState,
} from "@/features/goal/application/goalReadModel";
import { goalCanResume } from "@/features/goal/application/goalStatusPresentation";
import { type StartGoalInput } from "@/features/goal/application/ports/goalCommandsGateway";
import {
  GoalComposerModeOwner,
  type GoalComposerSubmission,
} from "@/features/goal/application/goalComposerMode";
interface GoalComposerSubmitDependencies {
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
      return (
        draft.slash?.command === GOAL_SLASH_COMMAND || Boolean(sessionId && owner.active(sessionId))
      );
    },
    submit: (context) => submitGoalComposerMode(owner, dependencies, context),
  };
}

// Goal mode accepts plain text for an active Session whose Goal can start or resume.
function admitGoalSubmission(
  owner: GoalComposerModeOwner,
  dependencies: GoalComposerSubmitDependencies,
  context: ComposerSubmitModeContext,
) {
  const sessionId = dependencies.getActiveSessionId();
  const state =
    sessionId && !owner.retired && dependencies.runtimeAvailable()
      ? dependencies.goalState(sessionId)
      : undefined;
  if (!sessionId || state?.available !== true) {
    dependencies.reportUnavailable();
    return undefined;
  }
  if (context.hasImages || context.hasPastes || context.hasReferences) {
    dependencies.reportUnsupportedAttachments();
    return undefined;
  }
  if (!goalCanEnterComposerMode(state.goal)) {
    dependencies.reportUnavailable();
    return undefined;
  }
  return { sessionId, existing: state.goal };
}

function submitGoalComposerMode(
  owner: GoalComposerModeOwner,
  dependencies: GoalComposerSubmitDependencies,
  context: ComposerSubmitModeContext,
): void {
  const admitted = admitGoalSubmission(owner, dependencies, context);
  if (!admitted) return;
  const { sessionId, existing } = admitted;
  const slash = context.slash;
  const goalCommand = slash?.command === GOAL_SLASH_COMMAND;
  const objective = (goalCommand ? slash.args : context.text).trim();

  if (goalCommand && !owner.active(sessionId) && !owner.activate(sessionId)) return;

  if (!objective) {
    if (goalCommand) {
      context.clear();
      dependencies.focusComposer();
    }
    return;
  }

  let input: StartGoalInput;
  try {
    input = captureStartGoalInput(sessionId, objective, dependencies.runOptions());
  } catch (error) {
    if (!dependencies.retired(error)) dependencies.reportStartError(error);
    return;
  }
  const startCommand = dependencies.captureStart();
  const start = (submission: GoalComposerSubmission) => {
    void startGoal(submission, dependencies, startCommand, context, input);
  };
  if (existing) {
    owner.requestReplacement(sessionId, existing.objective, start);
    return;
  }
  const submission = owner.begin(sessionId);
  if (submission) start(submission);
}

function captureStartGoalInput(
  sessionId: string,
  objective: string,
  { provider, model, reasoningEffort }: AgentRunStartOptions,
): StartGoalInput {
  return {
    sessionId,
    objective,
    ...(provider !== undefined ? { provider } : {}),
    ...(model !== undefined ? { model } : {}),
    ...(reasoningEffort !== undefined ? { reasoningEffort } : {}),
  };
}

async function startGoal(
  submission: GoalComposerSubmission,
  dependencies: GoalComposerSubmitDependencies,
  startCommand: (input: StartGoalInput) => Promise<void>,
  context: ComposerSubmitModeContext,
  input: StartGoalInput,
): Promise<void> {
  try {
    await startCommand(input);
    const committed = submission.started();
    if (committed) context.accept();
  } catch (error) {
    submission.failed();
    if (!dependencies.retired(error)) dependencies.reportStartError(error);
  }
}

export function goalCanEnterComposerMode(goal: GoalReadModel | null): boolean {
  return !goal || goalCanResume(goal);
}
