import { type AgentAccess } from "@/features/agent/public/agent";
import { type RuntimeAccess } from "@/features/connection/public/runtime";
import {
  asSessionId,
  createMutationSettler,
  type Goal,
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
import { type GoalReadModel, type GoalState } from "@/features/goal/application/goalReadModel";
function goalMutationIdentity(
  method: "start" | "update" | "clear" | "stop" | "resume",
  input: unknown,
): string {
  return JSON.stringify([`goals.${method}`, input]);
}

export function toGoalReadModel(goal: Goal): GoalReadModel {
  return {
    sessionId: goal.sessionId,
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

export function toGoalCommandReceipt(goal: Pick<Goal, "sessionId">): GoalCommandReceipt {
  return { sessionId: goal.sessionId };
}

export function runtimeGoalMaterial(goal: Goal | undefined, available: boolean): GoalState {
  return {
    available,
    goal: available && goal ? toGoalReadModel(goal) : null,
  };
}

class RuntimeGoalCommandsGateway implements GoalCommandsGateway {
  readonly #client: FlameClient;
  #mutations: MutationSettler = createMutationSettler();

  constructor(client: FlameClient) {
    this.#client = client;
  }

  async start(input: Parameters<GoalCommandsGateway["start"]>[0]) {
    const goal = await this.#mutations.settle(goalMutationIdentity("start", input), (signal) =>
      this.#client.goals.start({ ...input, sessionId: asSessionId(input.sessionId) }, signal),
    );
    return toGoalCommandReceipt(goal);
  }

  async update(input: Parameters<GoalCommandsGateway["update"]>[0]) {
    const goal = await this.#mutations.settle(goalMutationIdentity("update", input), (signal) =>
      this.#client.goals.update({ ...input, sessionId: asSessionId(input.sessionId) }, signal),
    );
    return toGoalCommandReceipt(goal);
  }

  async clear(sessionId: string) {
    await this.#mutations.settle(goalMutationIdentity("clear", sessionId), (signal) =>
      this.#client.goals.clear(asSessionId(sessionId), signal),
    );
    return { sessionId };
  }

  async stop(sessionId: string) {
    const goal = await this.#mutations.settle(goalMutationIdentity("stop", sessionId), (signal) =>
      this.#client.goals.stop(asSessionId(sessionId), signal),
    );
    return toGoalCommandReceipt(goal);
  }

  async resume(sessionId: string) {
    const goal = await this.#mutations.settle(goalMutationIdentity("resume", sessionId), (signal) =>
      this.#client.goals.resume(asSessionId(sessionId), signal),
    );
    return toGoalCommandReceipt(goal);
  }

  dispose(): void {
    this.#mutations.dispose();
  }
}

export interface GoalRuntimeAdapter {
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
    "goal",
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
