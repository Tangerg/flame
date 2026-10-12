import { describe, expect, it } from "vitest";
import { applyTheme, captureTheme, validateTheme } from "../dist/theme.js";

describe("read-only appearance projections", () => {
  it("projects custom host tokens and updates the same guest document", () => {
    const root = document.documentElement;
    root.style.setProperty("--color-accent", "#7344aa");
    const initial = captureTheme(root, "dark", 18);
    const guest = document.createElement("section");
    applyTheme(guest, initial);
    expect(guest.style.getPropertyValue("--color-accent")).toBe("#7344aa");
    expect(guest.style.colorScheme).toBe("dark");
    expect(guest.dataset.flameUiFontSize).toBe("18");
    root.style.setProperty("--color-accent", "#226644");
    applyTheme(guest, captureTheme(root, "light", 11));
    expect(guest.style.getPropertyValue("--color-accent")).toBe("#226644");
    expect(guest.dataset.flameUiFontSize).toBe("11");
  });
  it("rejects an incomplete or foreign token set before changing the document", () => {
    const theme = captureTheme(document.documentElement, "light", 14);
    expect(() =>
      validateTheme({ ...theme, tokens: { "--foreign": "red" } }),
    ).toThrow("Invalid theme projection");
    expect(() => validateTheme({ ...theme, fontSize: 30 })).toThrow(
      "Invalid theme projection",
    );
    const target = document.createElement("div");
    expect(() => applyTheme(target, { ...theme, tokens: {} })).toThrow();
    expect(target.getAttribute("style")).toBeNull();
  });
});
