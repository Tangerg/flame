import { type AgentAccess } from "@/features/agent/public/agent";
import { type RuntimeServerScope } from "@/features/connection/public/services";
import {
  asSessionId,
  createMutationSettler,
  type Goal,
  type GoalTarget,
  type FlameClient,
  type SessionSnapshot,
  type MutationSettler,
} from "@flame/runtime-client";
import { type GoalCommandsGateway } from "@/features/goal/application/ports/goalCommandsGateway";
import { type GoalCommandOwner } from "@/features/goal/application/goalCommands";
import { GOAL_MATERIAL, type GoalReadModel } from "@/features/goal/application/goalReadModel";
function toGoalReadModel(goal: Goal): GoalReadModel {
  return {
    sessionId: goal.sessionId,
    incarnationId: goal.incarnationId,
    ...(goal.author ? { author: goal.author } : {}),
    objective: goal.objective,
    status: goal.status,
    ...(goal.reason ? { reason: { ...goal.reason } } : {}),
    used: {
      runs: goal.used.runs,
      ...(goal.used.costUsd !== undefined ? { costUsd: goal.used.costUsd } : {}),
      steps: goal.used.steps,
    },
    createdAt: goal.createdAt,
  };
}

function assertGoalSession(expectedSessionId: string, goal: Goal): void {
  if (goal.sessionId !== expectedSessionId) {
    throw new Error(
      `goal command committed to session ${goal.sessionId}, expected ${expectedSessionId}`,
    );
  }
}

class RuntimeGoalCommandsGateway implements GoalCommandsGateway {
  readonly #client: FlameClient;
  readonly #mutations: MutationSettler = createMutationSettler();

  constructor(client: FlameClient) {
    this.#client = client;
  }

  async start(input: Parameters<GoalCommandsGateway["start"]>[0]) {
    const goal = await this.#mutations.settle((signal) =>
      this.#client.goals.start({ ...input, sessionId: asSessionId(input.sessionId) }, signal),
    );
    assertGoalSession(input.sessionId, goal);
  }

  async update(input: Parameters<GoalCommandsGateway["update"]>[0]) {
    const goal = await this.#mutations.settle((signal) => this.#client.goals.update(input, signal));
    assertGoalSession(input.sessionId, goal);
  }

  clear(target: GoalTarget) {
    return this.#mutations.settle((signal) => this.#client.goals.clear(target, signal));
  }

  stop(target: GoalTarget) {
    return this.#mutations.settle((signal) => this.#client.goals.stop(target, signal));
  }

  async resume(target: GoalTarget) {
    const goal = await this.#mutations.settle((signal) =>
      this.#client.goals.resume(target, signal),
    );
    assertGoalSession(target.sessionId, goal);
  }

  dispose(): void {
    this.#mutations.dispose();
  }
}

export function bindGoalRuntime(
  agentAccess: AgentAccess,
  runtimeClient: () => FlameClient,
  commandOwner: GoalCommandOwner,
  scope: RuntimeServerScope,
): () => void {
  const replaceTarget = () =>
    commandOwner.replaceTarget(() => new RuntimeGoalCommandsGateway(runtimeClient()));
  replaceTarget();
  const unsubscribe = scope.subscribeReplacement(replaceTarget);
  const disposeSharedMaterial = agentAccess.registerAgentSessionSharedMaterial<SessionSnapshot>(
    GOAL_MATERIAL,
    (_sessionId, snapshot) => (snapshot.goal ? toGoalReadModel(snapshot.goal) : null),
  );
  return () => {
    unsubscribe();
    disposeSharedMaterial();
    commandOwner.retireTarget();
  };
}
