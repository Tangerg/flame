import { type AgentRunStartOptions } from "@/features/agent/public/runOptions";
import {
  type SlashCommandContext,
  type SlashCommandHandler,
} from "@/features/composer/public/types";
import { type GoalMaterial } from "@/features/goal/application/goalReadModel";
import { goalCanResume } from "@/features/goal/application/goalStatusPresentation";
import type { StartGoalRequest } from "@flame/runtime-client/wire";
import {
  type CompleteGoalSubmission,
  type GoalComposerSubmissionOwner,
} from "@/features/goal/application/goalComposerSubmission";
interface GoalComposerCommandDependencies {
  getActiveSessionId(): string | null;
  readGoal(sessionId: string): GoalMaterial | undefined;
  canStart(existing: GoalMaterial): boolean;
  runOptions(): AgentRunStartOptions;
  captureStart(): (input: StartGoalRequest) => Promise<void>;
  prepareObjective(): void;
  reportUnavailable(): void;
  reportUnsupportedAttachments(): void;
  reportStartError(error: unknown): void;
  retired(error: unknown): boolean;
}

export function createGoalComposerCommand(
  owner: GoalComposerSubmissionOwner,
  dependencies: GoalComposerCommandDependencies,
): SlashCommandHandler {
  return (context) => submitGoalCommand(owner, dependencies, context);
}

function admitGoalSubmission(
  dependencies: GoalComposerCommandDependencies,
  context: SlashCommandContext,
) {
  const sessionId = dependencies.getActiveSessionId();
  const existing = sessionId ? dependencies.readGoal(sessionId) : undefined;
  if (!sessionId || existing === undefined || !dependencies.canStart(existing)) {
    dependencies.reportUnavailable();
    return undefined;
  }
  if (context.hasAttachments) {
    dependencies.reportUnsupportedAttachments();
    return undefined;
  }
  if (existing && !goalCanResume(existing)) {
    dependencies.reportUnavailable();
    return undefined;
  }
  return { sessionId, existing };
}

function submitGoalCommand(
  owner: GoalComposerSubmissionOwner,
  dependencies: GoalComposerCommandDependencies,
  context: SlashCommandContext,
): void {
  const objective = context.args.trim();
  if (!objective) {
    dependencies.prepareObjective();
    return;
  }
  const admitted = admitGoalSubmission(dependencies, context);
  if (!admitted) return;
  const { sessionId, existing } = admitted;

  let input: StartGoalRequest;
  try {
    input = captureStartGoalRequest(sessionId, objective, dependencies.runOptions(), existing);
  } catch (error) {
    if (!dependencies.retired(error)) dependencies.reportStartError(error);
    return;
  }
  const startCommand = dependencies.captureStart();
  const start = (complete: CompleteGoalSubmission) => {
    void startGoal(complete, dependencies, startCommand, context, input);
  };
  if (existing) {
    owner.requestReplacement(sessionId, existing.objective, start);
    return;
  }
  const complete = owner.begin(sessionId);
  if (complete) start(complete);
}

function captureStartGoalRequest(
  sessionId: string,
  objective: string,
  { provider, model, reasoningEffort }: AgentRunStartOptions,
  existing: GoalMaterial,
): StartGoalRequest {
  return {
    sessionId,
    objective,
    ...(existing ? { replaceIncarnationId: existing.incarnationId } : {}),
    ...(provider !== undefined ? { provider } : {}),
    ...(model !== undefined ? { model } : {}),
    ...(reasoningEffort !== undefined ? { reasoningEffort } : {}),
  };
}

async function startGoal(
  complete: CompleteGoalSubmission,
  dependencies: GoalComposerCommandDependencies,
  startCommand: (input: StartGoalRequest) => Promise<void>,
  context: SlashCommandContext,
  input: StartGoalRequest,
): Promise<void> {
  try {
    await startCommand(input);
    if (complete()) context.accept();
  } catch (error) {
    complete();
    if (!dependencies.retired(error)) dependencies.reportStartError(error);
  }
}
