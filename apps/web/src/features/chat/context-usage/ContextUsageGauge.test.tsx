import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { cleanup, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ContextUsageGauge } from "@/features/chat/context-usage/ContextUsageGauge";
const model = vi.hoisted(() => ({
  contextTokens: 198_000,
  modelSelection: { provider: "provider-b", model: "shared-model", reasoningEffort: "high" },
}));

vi.mock("@/features/agent/public/run", () => ({
  useCurrentRootMaterial: () => model,
}));

vi.mock("@/features/agent/public/session", () => ({
  useActiveSessionId: () => "session-a",
  useAgentSessions: () => ({
    data: [{ id: "session-a", provider: "provider-a", model: "shared-model" }],
  }),
}));

vi.mock("@/features/providers/public/queries", () => ({
  useModels: () => ({
    data: [
      { provider: "provider-a", id: "shared-model", tokenLimits: { contextWindow: 100_000 } },
      { provider: "provider-b", id: "shared-model", tokenLimits: { contextWindow: 258_000 } },
    ],
  }),
}));

describe("ContextUsageGauge", () => {
  afterEach(cleanup);

  it("reads the active Run's exact provider/model window instead of the editable Session default", () => {
    testClient.render(<ContextUsageGauge />);

    expect(screen.getByRole("button", { name: "Context usage: 77%" })).toBeTruthy();
  });
});
