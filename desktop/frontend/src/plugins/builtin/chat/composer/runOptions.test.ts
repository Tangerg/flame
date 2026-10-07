import { afterEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import { AGENT_SESSIONS_KEY } from "@/plugins/builtin/agent/public/session";
import { MODELS_KEY, SelectableModel } from "@/plugins/builtin/providers/public/queries";
import { resolveAgentRunStartOptions } from "@/plugins/sdk";
import { loadPluginsForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";
import type { ComposerModelPreference } from "./public/modelPreference";
import { composerRunOptions } from "./runOptions";

const state = vi.hoisted(() => ({
  preference: { kind: "session" } as ComposerModelPreference,
}));

vi.mock("./public/modelPreference", () => ({
  selectedComposerModelPreference: () => state.preference,
  useComposerModelPreference: () => state.preference,
}));

vi.mock("@/plugins/builtin/agent/public/session", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/plugins/builtin/agent/public/session")>()),
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
  await loadPluginsForTest(composerRunOptions);
  return resolveAgentRunStartOptions();
}

describe("Composer run options", () => {
  it("leaves the Session model in charge when the chosen model left the catalog", async () => {
    expect(
      await sentOptions({ kind: "explicit", provider: "anthropic", model: "claude-retired" }),
    ).toEqual({});
  });

  it("sends the effort it shows when the chosen effort left the model", async () => {
    expect(
      await sentOptions({
        kind: "explicit",
        provider: "deepseek",
        model: "deepseek-v4-pro",
        reasoningEffort: "retired-level",
      }),
    ).toEqual({ provider: "deepseek", model: "deepseek-v4-pro", reasoningEffort: "high" });
  });
});
