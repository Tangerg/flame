import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it } from "vitest";
import appearanceSettings from "@/features/settings/appearance";
import { SettingsPage } from "@/features/settings/public/SettingsPage";
import { SETTINGS_PANE } from "@/features/settings/public/panes";
import { definePlugin } from "@/extensions/definePlugin";
import { loadPluginsForTest } from "@/test/kernel";

beforeEach(() => {
  testClient.theme.store.setState(testClient.theme.store.getInitialState());
});
afterEach(() => {
  cleanup();
  testClient.theme.store.setState(testClient.theme.store.getInitialState());
});

it("finds appearance by field and section names without leaving the current pane", async () => {
  await loadPluginsForTest(
    appearanceSettings,
    definePlugin({
      name: "test.paper-search",
      setup(ctx) {
        ctx.contribute(SETTINGS_PANE, {
          id: "other",
          label: "Other",
          order: -1,
          component: () => <div>Current settings</div>,
        });
      },
    }),
  );
  testClient.navigation.go({ settings: "other" });
  testClient.render(<SettingsPage />);
  const search = screen.getByRole("searchbox");
  for (const value of [
    "Paper texture",
    "Texture strength",
    "Theme and color",
    "Typography",
    "Interaction",
    "Language",
  ]) {
    fireEvent.change(search, { target: { value } });
    expect(screen.getByRole("tab", { name: "Appearance" })).toBeTruthy();
    expect(screen.getByRole("tabpanel", { name: "Other" }).textContent).toContain(
      "Current settings",
    );
  }
});
