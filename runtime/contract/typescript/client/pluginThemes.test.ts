import { describe, expect, it } from "vitest";
import { validateWire } from "@flame/runtime-contract/validate";

const theme = { id: "sample", title: "Sample", scheme: "dark", colors: {} };

describe("plugin theme response admission", () => {
  it("accepts every portable color name with a six-digit hexadecimal value", () => {
    expect(
      validateWire("PluginTheme", {
        ...theme,
        colors: {
          background: "#102030",
          foreground: "#ABCDEF",
          accent: "#102030",
          muted: "#102030",
          border: "#102030",
        },
      }),
    ).toEqual([]);
  });

  it.each([
    { colors: { unexpected: "#102030" }, field: "unexpected" },
    { colors: { background: "url(https://example.invalid/image)" }, field: "background" },
    { colors: { accent: "#abc" }, field: "accent" },
    { colors: { border: "" }, field: "border" },
  ])("refuses invalid palette entries at the response boundary: $field", ({ colors, field }) => {
    const violations = validateWire("PluginTheme", { ...theme, colors });
    expect(violations).toHaveLength(1);
    expect(violations[0]?.path).toBe(`PluginTheme.colors["${field}"]`);
  });
});
