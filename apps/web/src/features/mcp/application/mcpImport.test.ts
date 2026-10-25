import { describe, expect, it, vi } from "vitest";
import { McpImportError, parseMcpImport } from "@/features/mcp/application/mcpImport";
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
    expect(thrown).toBeInstanceOf(McpImportError);
    expect(thrown).toMatchObject({ issue: { kind: "nameCollision", first, second, canonical } });
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
    expect(importFailure('{"mcpServers":{"empty":{}}}')).toMatchObject({
      issue: { kind: "transport", server: "empty" },
    });
  });
});

function importFailure(text: string): unknown {
  try {
    parseMcpImport(text);
  } catch (error) {
    return error;
  }
  throw new Error("Expected the source to be rejected before submission");
}

it.each([
  { entry: { type: "stdio" }, kind: "command" },
  { entry: { type: "stdio", command: "  " }, kind: "command" },
  { entry: { type: "streamableHttp" }, kind: "url" },
  { entry: { type: "sse", url: "  " }, kind: "url" },
])("preflights the active transport requirement: $kind", ({ entry, kind }) => {
  expect(
    importFailure(JSON.stringify({ mcpServers: { first: { command: "npx" }, second: entry } })),
  ).toMatchObject({ issue: { kind, server: "second" } });
});

it.each([0, -1, 1.5, Number.MAX_SAFE_INTEGER + 1])(
  "identifies the server with invalid timeout %s",
  (timeout) => {
    expect(
      importFailure(JSON.stringify({ mcpServers: { second: { command: "npx", timeout } } })),
    ).toMatchObject({ issue: { kind: "timeout", server: "second" } });
  },
);

it.each([
  { text: "{", issue: { kind: "json" } },
  { text: "{}", issue: { kind: "shape" } },
  { text: '{"mcpServers":{}}', issue: { kind: "empty" } },
  { text: '{"mcpServers":{"---":{"command":"npx"}}}', issue: { kind: "name", server: "---" } },
])("exposes a typed source issue for $text", ({ text, issue }) => {
  expect(importFailure(text)).toBeInstanceOf(McpImportError);
  expect(importFailure(text)).toMatchObject({ issue });
});

it("propagates unexpected JSON parser failures", () => {
  const failure = new Error("Unexpected parser failure");
  const parser = vi.spyOn(JSON, "parse").mockImplementationOnce(() => {
    throw failure;
  });
  try {
    expect(importFailure("{}")).toBe(failure);
  } finally {
    parser.mockRestore();
  }
});

it("preserves literal nonblank connection values during preflight", () => {
  const input = {
    mcpServers: {
      local: { type: "stdio", command: " node ", args: [" A B "] },
      remote: { type: "sse", url: " https://tools.example/Mcp?case=AbC " },
    },
  };
  const result = parseMcpImport(JSON.stringify(input));
  expect(result.servers[0]).toMatchObject({
    command: input.mcpServers.local.command,
    args: input.mcpServers.local.args,
  });
  expect(result.servers[1]).toMatchObject({ url: input.mcpServers.remote.url });
});
