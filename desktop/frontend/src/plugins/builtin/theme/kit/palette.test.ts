import { describe, expect, it } from "vitest";
import { colorThemeContribution } from "./colorThemeContribution";
import { paletteThemeContribution, SCHEME_BASE } from "./palette";

const builtinKeys = Object.keys(
  colorThemeContribution({
    id: "reference",
    label: "Reference",
    scheme: "dark",
    brand: { accent: "#3574f0", textOnAccent: "#ffffff" },
    surfaces: { bg: "#000000", surface: "#111111" },
    ink: { text: "#ffffff", textBright: "#ffffff" },
    borders: { border: "#222222", borderSoft: "#333333", divider: "#444444" },
    semantic: { negative: "#ff0000", warning: "#ffaa00", info: "#0000ff", success: "#00ff00" },
  }).tokens!,
).sort();

describe("palette themes", () => {
  it("derive the complete token set a built-in theme states from a partial palette", () => {
    const theme = paletteThemeContribution(
      { id: "partial", label: "Partial", scheme: "light", palette: { border: "#c0c0c0" } },
      "#3574f0",
      25,
    );

    expect(Object.keys(theme.tokens!).sort()).toEqual(builtinKeys);
    expect(theme.tokens!["color-bg"]).toBe(SCHEME_BASE.light.background);
    expect(theme.tokens!["color-text"]).toBe(SCHEME_BASE.light.foreground);
    expect(theme.tokens!["color-border"]).toBe("#c0c0c0");
  });

  it("declare an accent only when the palette states one", () => {
    const declared = paletteThemeContribution(
      { id: "declared", label: "Declared", scheme: "dark", palette: { accent: "#b4004e" } },
      "#3574f0",
      25,
    );
    const undeclared = paletteThemeContribution(
      { id: "undeclared", label: "Undeclared", scheme: "dark", palette: {} },
      "#3574f0",
      25,
    );

    expect(declared.accent).toBe("#b4004e");
    expect(declared.tokens!["color-accent"]).toBe("#b4004e");
    expect(undeclared.accent).toBeUndefined();
    expect(undeclared.tokens!["color-accent"]).toBe("#3574f0");
  });
});
