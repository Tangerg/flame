import { interfaceRequest } from "../contracts/fixture";
import { afterEach, expect, it } from "vitest";
import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import { InMemoryTransport } from "@modelcontextprotocol/sdk/inMemory.js";
import {
  Presentation,
  documentJSONSchema,
  requestJSONSchema,
} from "../contracts/document";
import { createBackend, PRESENT_TOOL, CATALOG_TOOL } from "./backend";

const cleanup: Array<() => Promise<void>> = [];
afterEach(async () => {
  for (const close of cleanup.splice(0).reverse()) await close();
});

async function connect() {
  const backend = createBackend();
  const client = new Client({ name: "genui-test", version: "1" });
  const [serverTransport, clientTransport] =
    InMemoryTransport.createLinkedPair();
  const serving = backend.serve(serverTransport);
  await client.connect(clientTransport);
  cleanup.push(async () => {
    await client.close();
    await backend.close();
    await serving;
  });
  return client;
}

const report = interfaceRequest();
it("advertises the protocol entry and trusted catalog", async () => {
  const client = await connect();
  const tools = (await client.listTools()).tools;
  expect(tools.map((tool) => tool.name)).toEqual([PRESENT_TOOL, CATALOG_TOOL]);
  expect(tools[0]).toMatchObject({
    inputSchema: requestJSONSchema,
    outputSchema: documentJSONSchema,
  });
  const result = await client.callTool({
    name: CATALOG_TOOL,
    arguments: { kind: "catalog" },
  });
  expect(result.isError).not.toBe(true);
  expect(result.structuredContent).toHaveProperty("catalog");
  expect(JSON.stringify(result.structuredContent)).toContain("TextField");
});
it("retains the accepted protocol messages and exact quantities", async () => {
  const client = await connect();
  const result = await client.callTool({
    name: PRESENT_TOOL,
    arguments: report,
  });
  expect(result.isError).not.toBe(true);
  expect(result.structuredContent).toEqual(
    Presentation.accept(report).snapshot(),
  );
  expect(JSON.stringify(result)).toContain("9007199254740993");
});
it("refuses an old document without accepting a replacement result", async () => {
  const client = await connect();
  await expect(
    client.callTool({
      name: PRESENT_TOOL,
      arguments: {
        title: "Old",
        nodes: [],
        state: [],
        actions: [],
        operations: [],
      },
    }),
  ).rejects.toThrow("schema");
});
