import { cleanup, render, screen } from "@testing-library/react";
import { afterAll, afterEach, beforeAll, expect, it } from "vitest";
import { defaultAccents } from "@/plugins/builtin/defaults";
import { installAppearancePreferencePort } from "@/plugins/builtin/theme/adapters/appearancePreferenceBinding";
import { useAppearanceStore } from "@/plugins/builtin/theme/adapters/appearanceStore";
import { WCAG_AA_TEXT, contrastRatio } from "@/plugins/builtin/theme/kit/legibility";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";
import { AccentSection } from "@/plugins/builtin/settings/appearance/ui/AccentSection";

let uninstall: () => void;
beforeAll(() => {
  uninstall = installAppearancePreferencePort();
});
afterAll(() => uninstall());

afterEach(() => {
  cleanup();
  useAppearanceStore.setState(useAppearanceStore.getInitialState());
});

it.each([
  ["#fb6e30", "Accent: Flame"],
  ["#0485f7", "Accent: Blue"],
  ["#7f52ff", "Accent: Purple"],
  ["#ffe066", "Pick a custom color"],
  ["#101828", "Pick a custom color"],
])(
  "draws the selection check in ink that reads on the swatch it marks (%s)",
  async (accent, name) => {
    await loadPluginsForTest(defaultAccents);
    useAppearanceStore.setState({ accent });
    render(<AccentSection />);
    const selected = screen
      .getAllByLabelText(name)
      .find((node) => node.tagName !== "INPUT") as HTMLElement;
    const swatch = selected.querySelector<HTMLElement>("span[aria-hidden][style]")!;
    const mark = selected.querySelector<HTMLElement>("[data-slot='accent-selection-mark']")!;
    expect(useAppearanceStore.getState().accent).toBe(accent);
    expect(mark.style.color).not.toBe("");
    expect(contrastRatio(mark.style.color, swatch.style.background)).toBeGreaterThanOrEqual(
      WCAG_AA_TEXT,
    );
  },
);
