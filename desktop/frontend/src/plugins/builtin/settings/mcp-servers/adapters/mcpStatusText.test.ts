import { describe, expect, it } from "vitest";
import { validateWire } from "@flame/runtime-contract/validate";
import { en } from "@/lib/i18n/locales/en";
import { MCP_STATUS_TYPES, mcpStatusText } from "./mcpStatusText";

describe("MCP status copy", () => {
  it("names only categories the MCP status contract can carry", () => {
    const phantom = MCP_STATUS_TYPES.filter(
      (type) => validateWire("MCPStatusProblem", { type }).length > 0,
    );
    expect(phantom).toEqual([]);
  });

  it("has copy for every category", () => {
    for (const type of MCP_STATUS_TYPES) {
      expect(en[`mcpStatus.${type}`]).toBeTruthy();
      expect(mcpStatusText(type)).not.toBe(`mcpStatus.${type}`);
    }
  });

  it("keeps no copy for a category it does not name", () => {
    const named = new Set(MCP_STATUS_TYPES.map((type) => `mcpStatus.${type}`));
    expect(
      Object.keys(en).filter((key) => key.startsWith("mcpStatus.") && !named.has(key)),
    ).toEqual([]);
  });
});
