import { createTheme, type Theme } from "@/features/theme/adapters/theme";
import { ThemeProvider } from "@/features/theme/public/theme";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, expect, it } from "vitest";
import { PaperTextureSection } from "@/features/theme/public/preferences/PaperTextureSection";

let owner: Theme;

beforeEach(() => {
  owner = createTheme(document, window);
});

afterEach(() => {
  cleanup();
  owner.dispose();
});

it("selects paper immediately and retains its strength across Off", () => {
  render(
    <ThemeProvider value={owner.access}>
      <PaperTextureSection />
    </ThemeProvider>,
  );
  const options = screen.getByRole("tablist", { name: "Paper texture" });
  expect(within(options).getByRole("tab", { name: "Off" }).getAttribute("aria-selected")).toBe(
    "true",
  );
  expect(screen.queryByRole("slider", { name: "Texture strength" })).toBeNull();
  fireEvent.click(within(options).getByRole("tab", { name: "Xuan paper" }));
  expect(owner.store.getState().paperTexture).toBe("xuan");
  const strength = screen.getByRole<HTMLInputElement>("slider", { name: "Texture strength" });
  expect(strength.min).toBe("1");
  expect(strength.max).toBe("5");
  act(() => strength.focus());
  fireEvent.keyDown(strength, { key: "ArrowRight" });
  expect(owner.store.getState().paperTextureStrength).toBe(4);
  expect(strength.value).toBe("4");
  fireEvent.click(within(options).getByRole("tab", { name: "Off" }));
  expect(screen.queryByRole("slider", { name: "Texture strength" })).toBeNull();
  fireEvent.click(within(options).getByRole("tab", { name: "Cotton paper" }));
  expect(owner.store.getState().paperTexture).toBe("cotton");
  expect(screen.getByRole<HTMLInputElement>("slider", { name: "Texture strength" }).value).toBe(
    "4",
  );
});
