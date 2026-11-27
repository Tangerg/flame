import { type AgentAccess } from "@/features/agent/public/agent";
import { type RuntimeAccess } from "@/features/connection/public/runtime";
import {
  asSessionId,
  createMutationSettler,
  type Goal,
  type GoalTarget,
  type FlameClient,
  type SessionSnapshot,
  type MutationSettler,
} from "@flame/runtime-client";
import { runtimeCapability } from "@/features/connection/public/capabilities";

import {
  type GoalCommandReceipt,
  type GoalCommandsGateway,
} from "@/features/goal/application/ports/goalCommandsGateway";
import { type GoalCommandOwner } from "@/features/goal/application/goalCommands";
import {
  GOAL_MATERIAL,
  type GoalReadModel,
  type GoalState,
} from "@/features/goal/application/goalReadModel";
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

function toGoalCommandReceipt(goal: Pick<Goal, "sessionId">): GoalCommandReceipt {
  return { sessionId: goal.sessionId };
}

function runtimeGoalMaterial(goal: Goal | undefined, available: boolean): GoalState {
  return {
    available,
    goal: goal ? toGoalReadModel(goal) : null,
  };
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
    return toGoalCommandReceipt(goal);
  }

  async update(input: Parameters<GoalCommandsGateway["update"]>[0]) {
    const goal = await this.#mutations.settle((signal) => this.#client.goals.update(input, signal));
    return toGoalCommandReceipt(goal);
  }

  async clear(target: GoalTarget) {
    await this.#mutations.settle((signal) => this.#client.goals.clear(target, signal));
    return { sessionId: target.sessionId };
  }

  async stop(target: GoalTarget) {
    await this.#mutations.settle((signal) => this.#client.goals.stop(target, signal));
    return { sessionId: target.sessionId };
  }

  async resume(target: GoalTarget) {
    const goal = await this.#mutations.settle((signal) =>
      this.#client.goals.resume(target, signal),
    );
    return toGoalCommandReceipt(goal);
  }

  dispose(): void {
    this.#mutations.dispose();
  }
}

interface GoalRuntimeAdapter {
  replaceRuntimeGeneration(): void;
  retireRuntimeGeneration(): void;
  dispose(): void;
}

export function createGoalRuntimeAdapter(
  agentAccess: AgentAccess,
  runtimeAccess: RuntimeAccess,
  runtimeClient: () => FlameClient,
  commandOwner: GoalCommandOwner,
  hasRuntimeGeneration: boolean,
): GoalRuntimeAdapter {
  let gateway: RuntimeGoalCommandsGateway | null = null;
  if (hasRuntimeGeneration) {
    commandOwner.replaceRuntimeGeneration(
      () => (gateway = new RuntimeGoalCommandsGateway(runtimeClient())),
    );
  }
  const disposeSharedMaterial = agentAccess.registerAgentSessionSharedMaterial<SessionSnapshot>(
    GOAL_MATERIAL,
    (_sessionId, snapshot) => {
      return runtimeGoalMaterial(snapshot.goal, runtimeCapability(runtimeAccess, "goals"));
    },
  );
  return {
    replaceRuntimeGeneration() {
      const predecessor = gateway;
      if (
        !commandOwner.replaceRuntimeGeneration(
          () => (gateway = new RuntimeGoalCommandsGateway(runtimeClient())),
        )
      )
        return;
      predecessor?.dispose();
    },
    retireRuntimeGeneration() {
      if (!commandOwner.retireRuntimeGeneration()) return;
      const predecessor = gateway;
      gateway = null;
      predecessor?.dispose();
    },
    dispose() {
      disposeSharedMaterial();
      commandOwner.retireRuntimeGeneration();
      gateway?.dispose();
      gateway = null;
    },
  };
}
