import { describe, expect, it } from "vitest";
import { McpImportNameCollision, parseMcpImport } from "./mcpImport";

describe("parseMcpImport", () => {
  it("normalizes stdio and remote servers into MCP config inputs", () => {
    const result = parseMcpImport(
      JSON.stringify({
        mcpServers: {
          git: {
            command: "npx",
            args: ["-y", "@modelcontextprotocol/server-git"],
            env: ["TOKEN=a=b", "EMPTY"],
            cwd: "/repo",
          },
          cloud: {
            type: "sse",
            url: "https://example.com/mcp",
            headers: {
              Authorization: "Bearer secret",
              "X-Trace": "abc",
            },
            timeout: 30,
          },
        },
      }),
    );

    expect(result.servers).toEqual([
      {
        name: "git",
        transport: "stdio",
        enabled: true,
        command: "npx",
        args: ["-y", "@modelcontextprotocol/server-git"],
        env: { TOKEN: "a=b", EMPTY: "" },
        dir: "/repo",
        handshakeTimeout: { type: "unbounded" },
      },
      {
        name: "cloud",
        transport: "streamableHttp",
        enabled: true,
        url: "https://example.com/mcp",
        authorization: "Bearer secret",
        headers: { "X-Trace": "abc" },
        handshakeTimeout: { type: "bounded", seconds: 30 },
      },
    ]);
  });

  it.each([
    { kind: "case folding", first: "GitHub", second: "github", canonical: "github" },
    {
      kind: "punctuation cleanup",
      first: "docs tools",
      second: "_docs-tools_",
      canonical: "docs-tools",
    },
    {
      kind: "truncation",
      first: `${"a".repeat(32)}-one`,
      second: `${"a".repeat(32)}-two`,
      canonical: "a".repeat(32),
    },
  ])("rejects distinct source names that collide after $kind", ({ first, second, canonical }) => {
    const text = JSON.stringify({
      mcpServers: {
        [first]: { command: "first" },
        [second]: { url: "https://example.com/mcp" },
      },
    });

    const thrown = (() => {
      try {
        parseMcpImport(text);
      } catch (err) {
        return err;
      }
    })();
    expect(thrown).toBeInstanceOf(McpImportNameCollision);
    expect(thrown).toMatchObject({ first, second, canonical });
  });

  it("keeps order and reports renames when normalized names stay distinct", () => {
    const result = parseMcpImport(
      JSON.stringify({
        mcpServers: {
          "My Server": { command: "first" },
          "my-server-2": { command: "second" },
          Other: { url: "https://example.com/mcp" },
        },
      }),
    );

    expect(result.servers.map((server) => server.name)).toEqual([
      "my-server",
      "my-server-2",
      "other",
    ]);
    expect(result.renamed).toEqual([
      { from: "My Server", to: "my-server" },
      { from: "Other", to: "other" },
    ]);
  });

  it("rejects server entries that do not describe a transport", () => {
    expect(() => parseMcpImport('{"mcpServers":{"empty":{}}}')).toThrow(
      'Server "empty" has neither a command (stdio) nor a url (streamableHttp)',
    );
  });
});
