import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { defaultAccents } from "@/features/theme/defaultAccents";
import { ACCENT } from "@/features/theme/public/extensionPoints";
import { WCAG_AA_TEXT, contrastRatio } from "@/features/theme/kit/legibility";
import { contributeForTest, loadPluginsForTest } from "@/test/kernel";
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
    const trigger = screen.getByRole("button", { name });
    const selected = trigger.closest("[data-focus-proxy]") ?? trigger;
    const swatch = selected.querySelector<HTMLElement>("span[aria-hidden][style]")!;
    const mark = selected.querySelector<HTMLElement>("[data-slot='accent-selection-mark']")!;
    expect(testClient.theme.store.getState().accent).toBe(accent);
    expect(mark.style.color).not.toBe("");
    expect(contrastRatio(mark.style.color, swatch.style.background)).toBeGreaterThanOrEqual(
      WCAG_AA_TEXT,
    );
  },
);

function pressedPresets() {
  return screen
    .getAllByRole("button", { name: /^Accent: / })
    .filter((button) => button.getAttribute("aria-pressed") === "true")
    .map((button) => button.getAttribute("aria-label"));
}

function customPicker() {
  const picker = screen.getByRole("button", { name: "Pick a custom color" });
  const description = document.getElementById(picker.getAttribute("aria-describedby")!)!;
  const native = picker
    .closest("[data-focus-proxy]")!
    .querySelector<HTMLInputElement>("input[type='color']")!;
  return { description: description.textContent, value: native.value };
}

it.each([
  ["#FB6E30", "Accent: Flame"],
  ["#Fb6e30", "Accent: Flame"],
  ["#0485F7", "Accent: Blue"],
  ["#7f52Ff", "Accent: Purple"],
])("selects the built-in preset spelled %s", async (accent, name) => {
  await loadPluginsForTest(defaultAccents);
  testClient.theme.access.preference.edit().setAccent(accent);
  expect(testClient.theme.store.getState().accent).toBe(accent);
  testClient.render(<AccentSection />);
  expect(pressedPresets()).toEqual([name]);
  expect(testClient.theme.store.getState().accent).toBe(accent);
});

it.each([
  ["#1A9E8F", "#1a9e8f"],
  ["#1a9E8f", "#1A9e8F"],
])("selects a contributed preset %s for the current value %s", async (color, accent) => {
  await loadPluginsForTest(defaultAccents);
  await contributeForTest((ctx) =>
    ctx.contributions.add(ACCENT, { id: "teal", label: "Teal", color, order: 4 }),
  );
  testClient.theme.access.preference.edit().setAccent(accent);
  testClient.render(<AccentSection />);
  expect(pressedPresets()).toEqual(["Accent: Teal"]);
  expect(testClient.theme.store.getState().accent).toBe(accent);
});

it("keeps explicit preset choices after a differently cased selection", async () => {
  await loadPluginsForTest(defaultAccents);
  testClient.theme.access.preference.edit().setAccent("#FB6E30");
  testClient.render(<AccentSection />);
  fireEvent.click(screen.getByRole("button", { name: "Accent: Blue" }));
  expect(testClient.theme.store.getState().accent).toBe("#0485f7");
  expect(pressedPresets()).toEqual(["Accent: Blue"]);
  fireEvent.click(screen.getByRole("button", { name: "Accent: Flame" }));
  expect(testClient.theme.store.getState().accent).toBe("#fb6e30");
  expect(pressedPresets()).toEqual(["Accent: Flame"]);
});

it("leaves every preset unpressed for an unmatched custom color", async () => {
  await loadPluginsForTest(defaultAccents);
  testClient.theme.access.preference.edit().setAccent("#123abc");
  testClient.render(<AccentSection />);
  expect(pressedPresets()).toEqual([]);
  expect(customPicker()).toEqual({ description: "#123ABC", value: "#123abc" });
  expect(testClient.theme.store.getState().accent).toBe("#123abc");
});
