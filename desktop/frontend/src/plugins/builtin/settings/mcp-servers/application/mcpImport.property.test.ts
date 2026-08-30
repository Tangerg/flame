import { describe, expect, it } from "vitest";
import { forEachSeed } from "@/test/arbitrary";
import { parseMcpImport } from "./mcpImport";

// This reads a blob a person pasted out of another MCP client, so its input is
// literally anything. The contract is narrow and worth pinning: it either answers
// with servers the configure request can carry, or it throws a message the pane
// shows. What it must never do is answer with a server the wire would refuse —
// that failure lands later, at the request, where the pane cannot explain it.

const WIRE_NAME = /^[a-z0-9][a-z0-9._-]{0,31}$/;

function attempt(text: string): { servers: number; threw: boolean; names: string[] } {
  try {
    const result = parseMcpImport(text);
    return {
      servers: result.servers.length,
      threw: false,
      names: result.servers.map((s) => s.name),
    };
  } catch {
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

  it("never answers with a name the wire would refuse", () => {
    const refused: string[] = [];
    const check = (text: string) => {
      const { threw, names } = attempt(text);
      if (threw) return;
      for (const name of names) if (!WIRE_NAME.test(name)) refused.push(name);
    };
    check('{"mcpServers": {"A B": {"command": "x"}}}');
    check('{"mcpServers": {"": {"command": "x"}}}');
    check(`{"mcpServers": {"${"n".repeat(200)}": {"command": "x"}}}`);
    check('{"mcpServers": {"Git": {"command": "mcp-git"}}}');
    check('{"mcpServers": {"Brave Search": {"command": "mcp-brave"}}}');
    forEachSeed(200, (a) => {
      check(JSON.stringify({ mcpServers: { [a.text()]: { command: "x" } } }));
    });
    expect(refused).toEqual([]);
  });

  it("renames a client's display-cased name into one the wire accepts", () => {
    const result = parseMcpImport(
      '{"mcpServers": {"Brave Search": {"command": "mcp-brave"}, "git": {"command": "mcp-git"}}}',
    );
    expect(result.servers.map((server) => server.name)).toEqual(["brave-search", "git"]);
    expect(result.renamed).toEqual([{ from: "Brave Search", to: "brave-search" }]);
  });

  it("explains rather than importing a name nothing can be made of", () => {
    expect(() => parseMcpImport('{"mcpServers": {"---": {"command": "x"}}}')).toThrow(
      /no name the runtime can accept/,
    );
  });

  it("keeps one server per named entry it accepts", () => {
    const { servers, threw } = attempt(
      '{"mcpServers": {"git": {"command": "mcp-git"}, "docs": {"url": "https://example.test"}}}',
    );
    expect(threw).toBe(false);
    expect(servers).toBe(2);
  });
});
