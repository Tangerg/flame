import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, expect, it } from "vitest";
import { installAppearancePreferencePort } from "@/features/theme/adapters/appearancePreferenceBinding";
import { useAppearanceStore } from "@/features/theme/adapters/appearanceStore";
import { PaperTextureSection } from "@/features/theme/public/preferences/PaperTextureSection";

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
