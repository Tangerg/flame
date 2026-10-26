import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient({ runtimeClient: getRuntimeClient });
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { afterEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import { type FlameClient } from "@flame/runtime-client";
import { definePlugin } from "@/extensions/definePlugin";
import { loadPluginsForTest, resetKernelForTest } from "@/test/kernel";
import {
  RuntimeConnectionGeneration,
  RUNTIME_STREAM,
  RUNTIME_SERVER_SCOPE,
} from "@/features/connection/public/services";

import { forgetRules } from "@/features/agent/application/approvalPolicy";
import { APPROVAL_RULES_KEY } from "@/features/agent/application/approvalPolicyQueries";
import { createAgentBootstrapPlugin } from "@/features/agent/bootstrap/index";
import { rejected } from "@/test/rejected";
let runtimeClient: () => FlameClient = () => {
  throw new Error("Runtime test client is not configured");
};

const getRuntimeClient = () => runtimeClient();

afterEach(async () => {
  await resetKernelForTest();
  vi.restoreAllMocks();

  queryClient.removeQueries({ queryKey: [APPROVAL_RULES_KEY] });
});

describe("Agent bootstrap Runtime generation wiring", () => {
  it("retires an admitted approval command when the Runtime process generation changes", async () => {
    const retired = deferred();
    const forgetRule = vi.fn(() => retired.promise);
    runtimeClient = () => ({ approval: { forgetRule } }) as unknown as FlameClient;
    let generation = RuntimeConnectionGeneration.forProcess("runtime_1");
    testClient.projection.store.setState({ connectionGeneration: generation });
    const runtime = definePlugin({
      name: "test.runtime-generation",
      provides: { stream: RUNTIME_STREAM, scope: RUNTIME_SERVER_SCOPE },
      setup() {
        return {
          scope: testClient.scope,
          stream: testClient.stream,
        };
      },
    });
    await loadPluginsForTest(
      runtime,
      createAgentBootstrapPlugin(testClient.agent, getRuntimeClient),
    );

    const command = rejected(forgetRules(testClient.agent.dependencies, ["rule-1"]));
    await vi.waitFor(() => expect(forgetRule).toHaveBeenCalledOnce());

    generation = RuntimeConnectionGeneration.forProcess("runtime_2");
    testClient.projection.store.setState({ connectionGeneration: generation });
    await expect(command).resolves.toMatchObject({ message: "agent_command_owner_retired" });

    retired.resolve();
  });
});

function deferred() {
  let resolve!: () => void;
  const promise = new Promise<void>((settle) => {
    resolve = settle;
  });
  return { promise, resolve };
}
