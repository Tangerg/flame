import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient({ runtimeClient: getRuntimeClient });
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  asItemId,
  asRunId,
  asSegmentId,
  RpcTransportError,
  type FlameClient,
  type MutationPromise,
  type RunEvent,
} from "@flame/runtime-client";
import { definePlugin } from "@/extensions/definePlugin";
import { pickAgentSource } from "@/features/agent/adapters/runtimeSources";
import { loadPluginsForTest, resetKernelForTest } from "@/test/kernel";
import { RuntimeConnectionGeneration, RUNTIME_STREAM } from "@/features/connection/public/services";
import { createRpcAgentPlugin } from "@/features/agent/rpcAgent";
let runtimeClient: () => FlameClient = () => {
  throw new Error("Runtime test client is not configured");
};

const getRuntimeClient = () => runtimeClient();

vi.mock("@/features/agent/application/session/activeSession", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/agent/application/session/activeSession")>()),
  getActiveSessionId: () => "ses_1",
}));

afterEach(async () => {
  await resetKernelForTest();

  vi.restoreAllMocks();
});

describe("RPC Agent Runtime generation wiring", () => {
  it("never replays a predecessor Run opening through the successor generation", async () => {
    const predecessorFailure = new RpcTransportError("predecessor response was lost");
    const predecessorRetry = vi.fn(() =>
      mutation(
        Promise.resolve({
          result: {
            runId: asRunId("run_predecessor"),
            segmentId: asSegmentId("seg_predecessor"),
            userItemId: asItemId("item_predecessor"),
          },
          observation: { type: "stream" as const, events: noEvents() },
        }),
        "predecessor-opening",
      ),
    );
    const predecessorStart = vi.fn(
      () =>
        Object.assign(Promise.reject(predecessorFailure), {
          idempotencyKey: "predecessor-opening",
          retry: predecessorRetry,
        }) as ReturnType<FlameClient["runs"]["start"]>,
    );
    runtimeClient = () => ({ runs: { start: predecessorStart } }) as unknown as FlameClient;

    const runtime = new RuntimeGenerationFixture("test.rpc-agent-runtime-generation");
    await loadPluginsForTest(
      runtime.plugin,
      createRpcAgentPlugin(testClient.agent.access, getRuntimeClient),
    );
    const source = pickAgentSource();
    expect(source?.id).toBe("rpc");
    const driver = source!.factory();
    const input = [{ type: "text" as const, text: "same logical input" }];

    await expect(driver.start(input, {})).rejects.toBe(predecessorFailure);

    const successorStart = vi.fn(() =>
      mutation(
        Promise.resolve({
          result: {
            runId: asRunId("run_successor"),
            segmentId: asSegmentId("seg_successor"),
            userItemId: asItemId("item_successor"),
          },
          observation: { type: "stream" as const, events: noEvents() },
        }),
        "successor-opening",
      ),
    );
    runtimeClient = () => ({ runs: { start: successorStart } }) as unknown as FlameClient;
    runtime.replace("runtime_2");

    await expect(driver.start(input, {})).resolves.toMatchObject({
      result: { runId: "run_successor", segmentId: "seg_successor" },
    });
    expect(successorStart).toHaveBeenCalledOnce();
    expect(predecessorRetry).not.toHaveBeenCalled();
  });

  it("retires an accepted predecessor stream only when the Runtime identity changes", async () => {
    let acceptedSignal: AbortSignal | undefined;
    const start = vi.fn((_params, signal?: AbortSignal) => {
      acceptedSignal = signal;
      return mutation(
        Promise.resolve({
          result: {
            runId: asRunId("run_accepted"),
            segmentId: asSegmentId("seg_accepted"),
            userItemId: asItemId("item_accepted"),
          },
          observation: { type: "stream" as const, events: noEvents() },
        }),
        "accepted-opening",
      );
    });
    runtimeClient = () => ({ runs: { start } }) as unknown as FlameClient;

    const runtime = new RuntimeGenerationFixture("test.rpc-agent-accepted-stream-generation");
    await loadPluginsForTest(
      runtime.plugin,
      createRpcAgentPlugin(testClient.agent.access, getRuntimeClient),
    );
    await pickAgentSource()!
      .factory()
      .start([{ type: "text", text: "accepted" }], {});
    expect(acceptedSignal?.aborted).toBe(false);

    runtime.notify();
    expect(acceptedSignal?.aborted).toBe(false);

    runtime.replace("runtime_2");
    expect(acceptedSignal?.aborted).toBe(true);
  });
});

class RuntimeGenerationFixture {
  readonly #subscribers = new Set<() => void>();
  #generation = RuntimeConnectionGeneration.forProcess("runtime_1");
  readonly plugin;

  constructor(name: string) {
    this.plugin = definePlugin({
      name,
      provides: { stream: RUNTIME_STREAM },
      setup: () => ({
        stream: {
          connectionGeneration: () => this.#generation,
          subscribeConnection: (onChange: () => void) => {
            this.#subscribers.add(onChange);
            return () => this.#subscribers.delete(onChange);
          },
          reportConnectionLoss: vi.fn(),
        },
      }),
    });
  }

  notify(): void {
    for (const subscriber of this.#subscribers) subscriber();
  }

  replace(generation: string): void {
    this.#generation = RuntimeConnectionGeneration.forProcess(generation);
    this.notify();
  }
}

function mutation<T>(promise: Promise<T>, idempotencyKey: string): MutationPromise<T> {
  return Object.assign(promise, {
    idempotencyKey,
    retry: vi.fn(),
  });
}

async function* noEvents(): AsyncIterable<RunEvent> {}
