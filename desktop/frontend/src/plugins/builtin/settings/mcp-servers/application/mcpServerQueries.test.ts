import { describe, expect, it } from "vitest";
import { mcpServerIcon } from "./mcpServerQueries";

// The protocol constrains an MCP server name to this shape, so any key the icon
// table is written in has to be reachable by a name the runtime can actually send.
const WIRE_NAME = /^[a-z0-9][a-z0-9._-]{0,31}$/;

describe("the MCP server glyph", () => {
  it("answers on names the wire permits", () => {
    for (const name of ["git", "filesystem", "shell", "slack", "github", "linear"]) {
      expect(name).toMatch(WIRE_NAME);
      expect(mcpServerIcon(name)).not.toBe("tool");
    }
  });

  it("falls back for a server it has never heard of", () => {
    expect(mcpServerIcon("sentry")).toBe("tool");
  });

  it("is not written in a casing the wire cannot produce", () => {
    expect(mcpServerIcon("Git")).toBe(mcpServerIcon("git"));
  });

  // `constructor` satisfies that regex, so it is a name a person can actually give a
  // server — and an object-literal table would have answered it with the Object
  // constructor, typed as the string this returns.
  it("falls back for a name that also names an inherited member", () => {
    expect("constructor").toMatch(WIRE_NAME);
    for (const name of ["constructor", "tostring", "valueof", "hasownproperty"]) {
      expect(mcpServerIcon(name)).toBe("tool");
    }
  });
});
