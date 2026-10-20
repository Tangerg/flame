import { type AgentRunStartOptions } from "@/features/agent/public/runOptions";
import {
  type ComposerSubmitModeContext,
  type ComposerSubmitModeSpec,
} from "@/features/composer/public/types";
import { type GoalReadModel, type GoalState } from "@/features/goal/application/goalReadModel";
import { type StartGoalInput } from "@/features/goal/application/ports/goalCommandsGateway";
import { GoalComposerModeOwner } from "@/features/goal/application/goalComposerMode";
export interface GoalComposerSubmitDependencies {
  getActiveSessionId(): string | null;
  goalState(sessionId: string): GoalState | undefined;
  runtimeAvailable(): boolean;
  runOptions(): AgentRunStartOptions;
  start(input: StartGoalInput): Promise<void>;
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
  if (!sessionId || !owner.ownsPublication() || !dependencies.runtimeAvailable()) {
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

  const start = () => {
    void startGoal(owner, dependencies, context, sessionId, objective);
  };
  if (existing) {
    owner.requestReplacement(sessionId, existing.objective, start);
    return;
  }
  if (!owner.begin(sessionId)) return;
  start();
}

async function startGoal(
  owner: GoalComposerModeOwner,
  dependencies: GoalComposerSubmitDependencies,
  context: ComposerSubmitModeContext,
  sessionId: string,
  objective: string,
): Promise<void> {
  try {
    await dependencies.start({ sessionId, objective, ...dependencies.runOptions() });
    const committed = owner.finish(sessionId, true);
    if (committed) context.accept();
  } catch (error) {
    owner.finish(sessionId, false);
    if (!dependencies.retired(error)) dependencies.reportStartError(error);
  }
}

export function goalCanEnterComposerMode(goal: GoalReadModel | null): boolean {
  return !goal || goal.status === "paused" || goal.status === "blocked";
}
