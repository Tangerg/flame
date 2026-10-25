import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it } from "vitest";
import { installAppearancePreferencePort } from "@/features/theme/adapters/appearancePreferenceBinding";
import { useAppearanceStore } from "@/features/theme/adapters/appearanceStore";
import appearanceSettings from "@/features/settings/appearance";
import { SettingsPage } from "@/features/settings/public/SettingsPage";
import { SETTINGS_PANE } from "@/features/settings/public/panes";
import { definePlugin } from "@/extensions/definePlugin";
import { loadPluginsForTest } from "@/test/kernel";
import { navigator } from "@/lib/navigation";

let uninstall: () => void;
beforeEach(() => {
  useAppearanceStore.setState(useAppearanceStore.getInitialState());
  uninstall = installAppearancePreferencePort();
});
afterEach(() => {
  cleanup();
  uninstall();
  useAppearanceStore.setState(useAppearanceStore.getInitialState());
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
  navigator().go({ settings: "other" });
  render(<SettingsPage />);
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
