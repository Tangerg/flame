import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { cleanup, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { defaultAccents } from "@/features/theme/defaultAccents";
import { WCAG_AA_TEXT, contrastRatio } from "@/features/theme/kit/legibility";
import { loadPluginsForTest } from "@/test/kernel";
import { AccentSection } from "@/features/theme/public/preferences/AccentSection";

afterEach(() => {
  cleanup();
  testClient.theme.store.setState(testClient.theme.store.getInitialState());
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
    testClient.theme.store.setState({ accent });
    testClient.render(<AccentSection />);
    const selected = screen
      .getAllByLabelText(name)
      .find((node) => node.tagName !== "INPUT") as HTMLElement;
    const swatch = selected.querySelector<HTMLElement>("span[aria-hidden][style]")!;
    const mark = selected.querySelector<HTMLElement>("[data-slot='accent-selection-mark']")!;
    expect(testClient.theme.store.getState().accent).toBe(accent);
    expect(mark.style.color).not.toBe("");
    expect(contrastRatio(mark.style.color, swatch.style.background)).toBeGreaterThanOrEqual(
      WCAG_AA_TEXT,
    );
  },
);
