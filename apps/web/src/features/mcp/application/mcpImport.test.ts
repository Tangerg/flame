import { describe, expect, it } from "vitest";
import { McpImportError, parseMcpImport } from "@/features/mcp/application/mcpImport";
describe("parseMcpImport", () => {
  it.each(["sse", "http", "streamableHTTP", "future-transport"])(
    "rejects the explicit unsupported transport %s before changing its protocol",
    (type) => {
      expect(
        importFailure(
          JSON.stringify({ mcpServers: { remote: { type, url: "https://example.test" } } }),
        ),
      ).toMatchObject({ issue: { kind: "unsupportedTransport", server: "remote" } });
    },
  );

  it.each(["Authorization", "authorization", "AUTHORIZATION", "AuThOrIzAtIoN"])(
    "preserves the credential from %s",
    (header) => {
      const result = parseMcpImport(
        JSON.stringify({
          mcpServers: {
            remote: {
              url: "https://example.test",
              headers: { [header]: "Bearer literal", "X-Key": "exact" },
            },
          },
        }),
      );
      expect(result[0]).toMatchObject({
        authorization: "Bearer literal",
        headers: { "X-Key": "exact" },
      });
    },
  );

  it("rejects invalid source identity without renaming it", () => {
    expect(importFailure('{"mcpServers":{"My Server":{"command":"node"}}}')).toMatchObject({
      issue: { kind: "name", server: "My Server" },
    });
  });

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
            type: "streamableHttp",
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

    expect(result).toEqual([
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

  it.each(["GitHub", "docs tools", "_docs-tools_", "a".repeat(33)])(
    "rejects invalid identity %s before creating a normalized name",
    (name) => {
      expect(
        importFailure(JSON.stringify({ mcpServers: { [name]: { command: "node" } } })),
      ).toMatchObject({ issue: { kind: "name", server: name } });
    },
  );

  it("keeps exact accepted names and entry order", () => {
    const servers = parseMcpImport(
      JSON.stringify({
        mcpServers: {
          "first._-": { command: "node" },
          second: { url: "https://example.test" },
        },
      }),
    );
    expect(servers.map((server) => server.name)).toEqual(["first._-", "second"]);
  });

  it.each([
    {
      headers: { Authorization: "first-secret", AUTHORIZATION: "second-secret" },
      header: "authorization",
    },
    {
      authorization: "first-secret",
      headers: { authorization: "second-secret" },
      header: "authorization",
    },
    { headers: { "X-Key": "first-secret", "x-key": "second-secret" }, header: "x-key" },
    { headers: { "X-Key": "same-secret", "x-key": "same-secret" }, header: "x-key" },
  ])(
    "rejects duplicate credential declarations for $header without exposing values",
    ({ header, ...source }) => {
      const failure = importFailure(
        JSON.stringify({
          mcpServers: {
            remote: {
              ...source,
              url: "https://example.test",
            },
          },
        }),
      );
      expect(failure).toMatchObject({
        issue: { kind: "headerConflict", server: "remote", header },
      });
      expect(String(failure)).not.toContain("secret");
      expect(JSON.stringify(failure)).not.toContain("secret");
    },
  );

  it.each([undefined, "stdio", "streamableHttp"])(
    "rejects ambiguous endpoints with type %s",
    (type) => {
      expect(
        importFailure(
          JSON.stringify({
            mcpServers: {
              ambiguous: {
                type,
                command: "node",
                url: "https://example.test",
              },
            },
          }),
        ),
      ).toMatchObject({ issue: { kind: "transport", server: "ambiguous" } });
    },
  );

  it.each([
    '{"mcpServers":{"same":{"command":"first"},"same":{"command":"second"}}}',
    '{"mcpServers":{"remote":{"url":"https://example.test","headers":{"X-Key":"first","X-Key":"second"}}}}',
  ])("rejects duplicate JSON members before losing evidence", (source) => {
    expect(importFailure(source)).toMatchObject({ issue: { kind: "json" } });
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
  { entry: { type: "stdio", command: "" }, kind: "command" },
  { entry: { type: "streamableHttp" }, kind: "url" },
  { entry: { type: "streamableHttp", url: "  " }, kind: "url" },
])("preflights the active transport requirement: $kind", ({ entry, kind }) => {
  expect(
    importFailure(JSON.stringify({ mcpServers: { first: { command: "npx" }, second: entry } })),
  ).toMatchObject({ issue: { kind, server: "second" } });
});

it.each([0, -1, 1.5, Number.MAX_SAFE_INTEGER + 1, 9_223_372_037, null, "30"])(
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

it("preserves literal nonblank connection values during preflight", () => {
  const input = {
    mcpServers: {
      local: { type: "stdio", command: " node ", args: [" A B "] },
      remote: { type: "streamableHttp", url: " https://tools.example/Mcp?case=AbC " },
    },
  };
  const result = parseMcpImport(JSON.stringify(input));
  expect(result[0]).toMatchObject({
    command: input.mcpServers.local.command,
    args: input.mcpServers.local.args,
  });
  expect(result[1]).toMatchObject({ url: input.mcpServers.remote.url });
});

it("preserves a whitespace-only executable and exact numeric timeout JSON", () => {
  expect(parseMcpImport('{"mcpServers":{"local":{"command":" ","timeout":1e1}}}')[0]).toMatchObject(
    { command: " ", handshakeTimeout: { type: "bounded", seconds: 10 } },
  );
});
