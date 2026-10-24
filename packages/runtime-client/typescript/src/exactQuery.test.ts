import { expect, it } from "vitest";
import { createFlameClient } from "./sdk";
import { createHttpTransport } from "./transports/http";
import { parseJSON, stringifyJSON } from "./json";
import { maximum } from "./validation/wireCheck";
import summary from "@flame/runtime-client/testing/samples/usageSummary.json";
import server from "@flame/runtime-client/testing/samples/mcpServer.json";

it("keeps original accounting numbers for archival queries and refuses lossy typed results", async () => {
  const value = JSON.stringify({ ...summary, total: { inputTokens: "WIDE", costUsd: "DECIMAL" } })
    .replace('"WIDE"', "9007199254740993")
    .replace('"DECIMAL"', "1.0000000000000001");
  let sends = 0;
  const client = createFlameClient(
    createHttpTransport({
      baseUrl: "http://host.test",
      fetch: (async (_url, options) => {
        sends++;
        const request = JSON.parse(options!.body as string) as { id: string };
        return new Response(
          '{"jsonrpc":"2.0","id":' + JSON.stringify(request.id) + ',"result":' + value + "}",
          { headers: { "Content-Type": "application/json" } },
        );
      }) as typeof fetch,
    }),
  );
  try {
    const raw = await client.queryJSON("usage.summary", {});
    expect(raw).toContain("9007199254740993");
    expect(raw).toContain("1.0000000000000001");
    await expect(client.usage.summary()).rejects.toThrow("number cannot be represented");
    expect(await client.queryJSON("usage.summary", {})).toBe(raw);
    await expect(client.queryJSON("sessions.create", {})).rejects.toThrow("requires a unary query");
    expect(sends).toBe(3);
  } finally {
    await client.close();
  }
});

it("validates exact machine bounds without rounding the bound", () => {
  const check = maximum("9223372036854775807");
  const out: Array<{ path: string; detail: string }> = [];
  check(parseJSON("9223372036854775807"), "$", out);
  expect(out).toEqual([]);
  check(parseJSON("9223372036854775808"), "$", out);
  expect(out).toHaveLength(1);
});

it("preserves opaque tool schemas while projecting declared DTO numbers", async () => {
  const schema = '{"maximum":9223372036854775807,"multipleOf":1.0,"examples":[1e+25]}';
  const tool = {
    name: "read",
    modelName: "example_read",
    nameConflicts: [],
    server: { origin: { type: "user" }, name: "example" },
    inputSchema: "SCHEMA",
  };
  const client = createFlameClient(
    createHttpTransport({
      baseUrl: "http://host.test",
      fetch: (async (_url, options) => {
        const request = JSON.parse(options!.body as string) as { id: string; method: string };
        const result =
          request.method === "mcp.servers.list"
            ? JSON.stringify({
                data: [{ ...server, status: { type: "connected", toolCount: "COUNT" } }],
                nextCursor: "",
              }).replace('"COUNT"', "1.0")
            : JSON.stringify({ data: [tool], nextCursor: "" }).replace('"SCHEMA"', schema);
        return new Response(
          '{"jsonrpc":"2.0","id":' + JSON.stringify(request.id) + ',"result":' + result + "}",
          { headers: { "Content-Type": "application/json" } },
        );
      }) as typeof fetch,
    }),
  );
  try {
    const result = await client.mcp.listTools();
    expect(stringifyJSON(result.data[0]!.inputSchema)).toBe(schema);
    const servers = await client.mcp.list();
    expect(servers.data[0]!.status).toEqual({ type: "connected", toolCount: 1 });
  } finally {
    await client.close();
  }
});
