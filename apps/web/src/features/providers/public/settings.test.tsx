import { beforeEach, afterEach, expect, it } from "vitest";
import { act, cleanup } from "@testing-library/react";
import { asyncDisposeSymbol } from "dougong";
import { createTestClient, type TestClient } from "@/test/renderClient";
import { contributeForTest, resetKernelForTest } from "@/test/kernel";
import { type ContributionLifetime } from "@/extensions/definePlugin";
import { SETTINGS_PANE } from "@/features/settings/public/panes";
import { useProviderSettingsPane } from "./settings";

let client: TestClient;
beforeEach(() => {
  client = createTestClient();
});
afterEach(async () => {
  cleanup();
  await resetKernelForTest();
  await client.dispose();
});

it("opens the exact installed settings entry and returns to management after withdrawal", async () => {
  const hook = client.renderHook(useProviderSettingsPane);
  expect(hook.result.current).toBe("plugins");
  let lifetime!: ContributionLifetime;
  await act(async () =>
    contributeForTest((context) => {
      lifetime = context.lifetime("model-settings");
      lifetime.contribute(SETTINGS_PANE, {
        id: "package:chosen:configuration",
        label: "Model settings",
        intent: "modelProviders",
        component: () => null,
      });
    }),
  );
  expect(hook.result.current).toBe("package:chosen:configuration");
  await act(async () => lifetime[asyncDisposeSymbol]());
  expect(hook.result.current).toBe("plugins");
});

it("requires an unambiguous declared intent instead of inferring from titles or artwork", async () => {
  await contributeForTest((context) => {
    context.contribute(SETTINGS_PANE, {
      id: "unrelated",
      label: "Providers",
      material: "providers",
      component: () => null,
    });
    for (const id of ["one", "two"])
      context.contribute(SETTINGS_PANE, {
        id,
        label: id,
        intent: "modelProviders",
        component: () => null,
      });
  });
  const hook = client.renderHook(useProviderSettingsPane);
  expect(hook.result.current).toBe("plugins");
});
