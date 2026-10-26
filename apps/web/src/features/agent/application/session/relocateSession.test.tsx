import { stubInstance } from "@/test/stubInstance";
import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { afterEach, describe, expect, it, vi } from "vitest";

import { queryClient } from "@/lib/queryClient";
import { type AgentRuntimeGateway } from "@/features/agent/application/runtimeGateway";
import { AGENT_SESSIONS_KEY } from "@/features/agent/application/session/sessionQueries";
import { useRelocateSession } from "@/features/agent/application/session/relocateSession";
let restoreRuntime: (() => void) | undefined;

afterEach(() => {
  restoreRuntime?.();
  restoreRuntime = undefined;
  vi.restoreAllMocks();
});

describe("useRelocateSession", () => {
  it("revalidates Session truth after an ambiguous conditional-write failure", async () => {
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    const invalidate = vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue();
    restoreRuntime = stubInstance(testClient.agent.dependencies.runtime, {
      updateSession: vi.fn().mockRejectedValue(new Error("response lost after commit")),
    } as unknown as AgentRuntimeGateway);
    const { result } = testClient.renderHook(() => useRelocateSession());

    await expect(result.current("ses_move", 4, "/next")).resolves.toBe(false);

    expect(invalidate).toHaveBeenCalledWith({ queryKey: [AGENT_SESSIONS_KEY] });
  });
});
