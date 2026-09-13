import { afterEach, describe, expect, it, vi } from "vitest";
import { AGENT_SESSIONS } from "@/plugins/builtin/agent/public/services";
import { definePlugin } from "@/plugins/sdk";
import { loadPluginsForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";
import { queryClient } from "@/lib/queryClient";
import { DATA_PROVIDER, SLASH_COMMAND } from "@/plugins/sdk/kernelPoints";
import { lookupExtensionByKey } from "@/plugins/sdk/selectors/extensions";
import { WORKSPACE_RECIPES_KEY } from "@/plugins/builtin/workspace/public/queries";
import recipesSlash from "./index";

afterEach(async () => {
  await resetKernelForTest();
});

describe("Recipe slash bootstrap", () => {
  it("reads setup-time session identity from its declared service", async () => {
    const activeSessionId = vi.fn(() => "");
    const subscribeActiveSessionId = vi.fn(() => () => undefined);
    const sessions = definePlugin({
      name: "test.recipe-session-ports",
      provides: { sessions: AGENT_SESSIONS },
      setup() {
        return {
          sessions: {
            getActiveSessionId: activeSessionId,
            getLifecycleSnapshot: () => ({ activeSessionId: "", openSessionIds: [] }),
            subscribeActiveSessionId,
            subscribeLifecycle: () => () => undefined,
          },
        };
      },
    });

    await loadPluginsForTest(recipesSlash, sessions);

    expect(activeSessionId).toHaveBeenCalled();
    expect(subscribeActiveSessionId).toHaveBeenCalledOnce();
  });
});

it("keeps slash recipes current when the workspace catalog is invalidated", async () => {
  let recipes = [
    {
      name: "review",
      body: "Review $ARGUMENTS",
      description: "Review code",
      argumentHint: "<path>",
    },
  ];
  await loadPluginsForTest(
    definePlugin({
      name: "test.recipe-catalog",
      provides: { sessions: AGENT_SESSIONS },
      setup(ctx) {
        ctx.contribute(DATA_PROVIDER, { key: WORKSPACE_RECIPES_KEY, fetcher: async () => recipes });
        return {
          sessions: {
            getActiveSessionId: () => "",
            getLifecycleSnapshot: () => ({ activeSessionId: "", openSessionIds: [] }),
            subscribeActiveSessionId: () => () => undefined,
            subscribeLifecycle: () => () => undefined,
          },
        };
      },
    }),
  );
  await loadPluginsForTest(recipesSlash);
  await vi.waitFor(() =>
    expect(lookupExtensionByKey(SLASH_COMMAND, "review")?.description).toBe("Review code  <path>"),
  );
  const send = vi.fn();
  await lookupExtensionByKey(SLASH_COMMAND, "review")?.run?.({ args: "client.go", send });
  expect(send).toHaveBeenCalledWith("Review client.go");

  recipes = [{ ...recipes[0]!, description: "Inspect changes", argumentHint: "<file>" }];
  await queryClient.invalidateQueries({ queryKey: [WORKSPACE_RECIPES_KEY] });
  await vi.waitFor(() =>
    expect(lookupExtensionByKey(SLASH_COMMAND, "review")?.description).toBe(
      "Inspect changes  <file>",
    ),
  );
  recipes = [];
  await queryClient.invalidateQueries({ queryKey: [WORKSPACE_RECIPES_KEY] });
  await vi.waitFor(() => expect(lookupExtensionByKey(SLASH_COMMAND, "review")).toBeUndefined());
});
