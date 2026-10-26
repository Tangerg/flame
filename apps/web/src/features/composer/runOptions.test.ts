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
import { AGENT_SESSIONS_KEY } from "@/features/agent/public/session";
import { MODELS_KEY } from "@/features/providers/public/queries";
import { SelectableModel } from "@/features/providers/public/queries";
import { resolveAgentRunStartOptions } from "@/features/agent/public/runOptions";
import { loadPluginsForTest, resetKernelForTest } from "@/test/kernel";
import { type ComposerModelPreference } from "@/features/composer/public/modelPreference";
import { composerRunOptions } from "@/features/composer/runOptions";
const state = vi.hoisted(() => ({
  preference: { kind: "session" } as ComposerModelPreference,
}));

vi.mock("@/features/composer/public/modelPreference", () => ({
  selectedComposerModelPreference: () => state.preference,
  useComposerModelPreference: () => state.preference,
}));

vi.mock("@/features/agent/public/session", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/agent/public/session")>()),
  getActiveSessionId: () => "session-1",
}));

afterEach(async () => {
  await resetKernelForTest();
  queryClient.clear();
  state.preference = { kind: "session" };
});

async function sentOptions(preference: ComposerModelPreference) {
  state.preference = preference;
  queryClient.setQueryData(
    [MODELS_KEY],
    [
      new SelectableModel({ provider: "openai", id: "gpt-5", label: "GPT-5" }),
      new SelectableModel({
        provider: "deepseek",
        id: "deepseek-v4-pro",
        label: "DeepSeek",
        reasoning: true,
        reasoningLevels: ["low", "high"],
        reasoningDefaultLevel: "high",
      }),
    ],
  );
  queryClient.setQueryData(
    [AGENT_SESSIONS_KEY],
    [{ id: "session-1", provider: "openai", model: "gpt-5" }],
  );
  await loadPluginsForTest(composerRunOptions(testClient.composer.access));
  return resolveAgentRunStartOptions();
}

describe("Composer run options", () => {
  it("sends the explicit model even when the catalog omits it", async () => {
    expect(
      await sentOptions({ kind: "explicit", provider: "anthropic", model: "claude-retired" }),
    ).toEqual({ provider: "anthropic", model: "claude-retired" });
  });

  it("preserves the authored effort when the catalog stops publishing it", async () => {
    expect(
      await sentOptions({
        kind: "explicit",
        provider: "deepseek",
        model: "deepseek-v4-pro",
        reasoningEffort: "retired-level",
      }),
    ).toEqual({ provider: "deepseek", model: "deepseek-v4-pro", reasoningEffort: "retired-level" });
    queryClient.removeQueries({ queryKey: [MODELS_KEY] });
    queryClient.removeQueries({ queryKey: [AGENT_SESSIONS_KEY] });
    expect(resolveAgentRunStartOptions()).toEqual({
      provider: "deepseek",
      model: "deepseek-v4-pro",
      reasoningEffort: "retired-level",
    });
  });
});
