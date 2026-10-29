import { describe, expect, it } from "vitest";
import { forEachSeed } from "@/test/arbitrary";
import { McpImportError, parseMcpImport } from "@/features/mcp/application/mcpImport";
import { validateWire } from "@flame/runtime-client/validation";

function attempt(text: string): { servers: number; threw: boolean; names: string[] } {
  try {
    const result = parseMcpImport(text);
    return {
      servers: result.length,
      threw: false,
      names: result.map((s) => s.name),
    };
  } catch (error) {
    if (!(error instanceof McpImportError)) throw error;
    return { servers: 0, threw: true, names: [] };
  }
}

describe("pasting an MCP client configuration", () => {
  it("either answers or explains, over arbitrary text", () => {
    forEachSeed(400, (a) => {
      expect(() => attempt(a.text())).not.toThrow();
    });
  });

  it("either answers or explains, over arbitrary JSON shapes", () => {
    const shapes = [
      "null",
      "0",
      '"text"',
      "[]",
      "{}",
      '{"mcpServers": null}',
      '{"mcpServers": []}',
      '{"mcpServers": {"a": null}}',
      '{"mcpServers": {"a": {}}}',
      '{"mcpServers": {"a": {"command": "x"}}}',
      '{"mcpServers": {"a": {"url": "https://example.test"}}}',
      '{"mcpServers": {"a": {"command": "x", "args": "not an array"}}}',
      '{"mcpServers": {"a": {"command": "x", "env": "not an object"}}}',
      '{"mcpServers": {"": {"command": "x"}}}',
      '{"servers": {"a": {"command": "x"}}}',
      '{"mcpServers": {"A B": {"command": "x"}}}',
      `{"mcpServers": {"${"n".repeat(200)}": {"command": "x"}}}`,
    ];
    for (const shape of shapes) expect(() => attempt(shape)).not.toThrow();
  });

  it("admits exactly the generated Runtime name contract without changing identity", () => {
    const check = (name: string) => {
      const accepted =
        validateWire("MCPServerID", { origin: { type: "user" }, name }, "request").length === 0;
      const result = attempt(JSON.stringify({ mcpServers: { [name]: { command: "x" } } }));
      expect(result.threw).toBe(!accepted);
      expect(result.names).toEqual(accepted ? [name] : []);
    };
    for (const name of ["A B", "", "Git", "Brave Search", "a._-", "0", "___", "__proto__"])
      check(name);
    forEachSeed(400, (a) => {
      check(a.text());
      check("a".repeat(a.int(256)));
      check(a.pick(["a", "A", "0", "_"]) + a.pick(["valid", "_x.y-z", " invalid", ""]));
    });
  });

  it("explains rather than importing a name nothing can be made of", () => {
    let failure: unknown;
    try {
      parseMcpImport('{"mcpServers": {"---": {"command": "x"}}}');
    } catch (error) {
      failure = error;
    }
    expect(failure).toMatchObject({ issue: { kind: "name", server: "---" } });
  });

  it("keeps one server per named entry it accepts", () => {
    const { servers, threw } = attempt(
      '{"mcpServers": {"git": {"command": "mcp-git"}, "docs": {"url": "https://example.test"}}}',
    );
    expect(threw).toBe(false);
    expect(servers).toBe(2);
  });
});
