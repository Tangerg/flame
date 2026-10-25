import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, expect, it } from "vitest";
import { installAppearancePreferencePort } from "@/features/theme/adapters/appearancePreferenceBinding";
import { useAppearanceStore } from "@/features/theme/adapters/appearanceStore";
import { PaperTextureSection } from "@/features/theme/public/preferences/PaperTextureSection";
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

it("selects paper immediately and retains its strength across Off", () => {
  render(<PaperTextureSection />);
  const options = screen.getByRole("tablist", { name: "Paper texture" });
  expect(within(options).getByRole("tab", { name: "Off" }).getAttribute("aria-selected")).toBe(
    "true",
  );
  expect(screen.queryByRole("slider", { name: "Texture strength" })).toBeNull();
  fireEvent.click(within(options).getByRole("tab", { name: "Xuan paper" }));
  expect(useAppearanceStore.getState().paperTexture).toBe("xuan");
  const strength = screen.getByRole<HTMLInputElement>("slider", { name: "Texture strength" });
  expect(strength.min).toBe("1");
  expect(strength.max).toBe("5");
  act(() => strength.focus());
  fireEvent.keyDown(strength, { key: "ArrowRight" });
  expect(useAppearanceStore.getState().paperTextureStrength).toBe(4);
  expect(strength.value).toBe("4");
  fireEvent.click(within(options).getByRole("tab", { name: "Off" }));
  expect(screen.queryByRole("slider", { name: "Texture strength" })).toBeNull();
  fireEvent.click(within(options).getByRole("tab", { name: "Cotton paper" }));
  expect(useAppearanceStore.getState().paperTexture).toBe("cotton");
  expect(screen.getByRole<HTMLInputElement>("slider", { name: "Texture strength" }).value).toBe(
    "4",
  );
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
