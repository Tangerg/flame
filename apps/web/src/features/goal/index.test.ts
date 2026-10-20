import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { afterEach, describe, expect, it, vi } from "vitest";
import { type Goal, type FlameClient, type MutationPromise } from "@flame/runtime-client";
import { definePlugin } from "@/extensions/definePlugin";
import { loadPluginsForTest, resetKernelForTest } from "@/test/kernel";
import {
  RuntimeConnectionGeneration,
  RUNTIME_STREAM,
  type RuntimeConnectionGeneration as RuntimeConnectionGenerationValue,
} from "@/features/connection/public/services";
import { resumeGoal, stopGoal } from "@/features/goal/application/goalCommands";
import { createGoalPlugin } from "@/features/goal/index";
import { rejected } from "@/test/rejected";
let runtimeClient: () => FlameClient = () => {
  throw new Error("Runtime test client is not configured");
};

const getRuntimeClient = () => runtimeClient();

const { synchronizeMountedAgentSession } = vi.hoisted(() => ({
  synchronizeMountedAgentSession: vi.fn().mockResolvedValue(true),
}));

vi.mock("@/features/agent/public/session", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/agent/public/session")>()),
  synchronizeMountedAgentSession,
}));

afterEach(async () => {
  await resetKernelForTest();

  synchronizeMountedAgentSession.mockClear();
  vi.restoreAllMocks();
});

describe("Goal plugin Runtime generation wiring", () => {
  it("retires predecessor commands and binds successor commands to the successor client", async () => {
    const retired = Promise.withResolvers<Goal>();
    const retiredStop = vi.fn(() => mutation(retired.promise, "retired-stop"));
    runtimeClient = () => ({ goals: { stop: retiredStop } }) as unknown as FlameClient;
    let generation = RuntimeConnectionGeneration.forProcess("runtime_1");
    const subscribers = new Set<() => void>();
    const runtime = definePlugin({
      name: "test.goal-runtime-generation",
      provides: { stream: RUNTIME_STREAM },
      setup() {
        return {
          stream: {
            connectionGeneration: () => generation,
            subscribeConnection(onChange: () => void) {
              subscribers.add(onChange);
              return () => subscribers.delete(onChange);
            },
            reportConnectionLoss: vi.fn(),
          },
        };
      },
    });
    await loadPluginsForTest(runtime, createGoalPlugin(getRuntimeClient));
    const predecessor = rejected(stopGoal("ses_goal"));
    await vi.waitFor(() => expect(retiredStop).toHaveBeenCalledOnce());

    const successorResume = vi.fn(() =>
      mutation(Promise.resolve(runtimeGoal("ses_goal")), "successor-resume"),
    );
    runtimeClient = () => ({ goals: { resume: successorResume } }) as unknown as FlameClient;
    generation = RuntimeConnectionGeneration.forProcess("runtime_2");
    for (const subscriber of subscribers) subscriber();

    await expect(predecessor).resolves.toMatchObject({
      message: "goal_command_generation_retired",
    });
    await expect(resumeGoal("ses_goal")).resolves.toBeUndefined();
    expect(successorResume).toHaveBeenCalledOnce();
    expect(synchronizeMountedAgentSession).toHaveBeenCalledOnce();

    retired.resolve(runtimeGoal("ses_goal"));
    await Promise.resolve();
    await Promise.resolve();
    expect(synchronizeMountedAgentSession).toHaveBeenCalledOnce();
  });

  it("does not construct a successor client when the Runtime connection is withdrawn", async () => {
    runtimeClient = () => ({ goals: {} }) as unknown as FlameClient;
    let generation: RuntimeConnectionGenerationValue | null =
      RuntimeConnectionGeneration.forProcess("runtime_1");
    const subscribers = new Set<() => void>();
    const runtime = definePlugin({
      name: "test.goal-runtime-withdrawal",
      provides: { stream: RUNTIME_STREAM },
      setup() {
        return {
          stream: {
            connectionGeneration: () => generation,
            subscribeConnection(onChange: () => void) {
              subscribers.add(onChange);
              return () => subscribers.delete(onChange);
            },
            reportConnectionLoss: vi.fn(),
          },
        };
      },
    });
    await loadPluginsForTest(runtime, createGoalPlugin(getRuntimeClient));

    runtimeClient = () => {
      throw new Error("Desktop container is closed");
    };
    generation = null;

    expect(() => {
      for (const subscriber of subscribers) subscriber();
    }).not.toThrow();

    expect(wasGenerationRetired(await rejected(stopGoal("ses_goal")))).toBe(true);

    const successorResume = vi.fn(() =>
      mutation(Promise.resolve(runtimeGoal("ses_goal")), "successor-resume"),
    );
    runtimeClient = () => ({ goals: { resume: successorResume } }) as unknown as FlameClient;
    generation = RuntimeConnectionGeneration.forProcess("runtime_2");
    for (const subscriber of subscribers) subscriber();

    await expect(resumeGoal("ses_goal")).resolves.toBeUndefined();
    expect(successorResume).toHaveBeenCalledOnce();
  });
});

function mutation<T>(promise: Promise<T>, idempotencyKey: string): MutationPromise<T> {
  return Object.assign(promise, {
    idempotencyKey,
    retry: vi.fn(),
  });
}

function runtimeGoal(sessionId: string): Goal {
  return {
    sessionId,
    objective: "Keep one generation",
    status: "active",
    provider: "openai",
    model: "gpt-5",
    used: { runs: 0, costUsd: 0, steps: 0 },
    createdAt: "2026-08-18T00:00:00Z",
    updatedAt: "2026-08-18T00:00:00Z",
  };
}
