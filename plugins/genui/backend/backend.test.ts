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

const report = {
  title: "Deployment review",
  state: [],
  actions: [],
  operations: [],
  nodes: [
    {
      id: "summary",
      type: "Text",
      props: { text: "Checked the recorded deployment." },
    },
    {
      id: "identifier",
      type: "Metric",
      props: { label: "Exact identifier", value: "9007199254740993" },
    },
  ],
};
const accepted = { ...report, bindings: [] };

it("publishes the document owner's schema for input and enriched retained result", async () => {
  const client = await connect();
  const { tools } = await client.listTools();
  expect(tools.map((tool) => tool.name)).toEqual([PRESENT_TOOL, CATALOG_TOOL]);
  expect(tools[0]).toMatchObject({
    name: PRESENT_TOOL,
    inputSchema: requestJSONSchema,
    outputSchema: documentJSONSchema,
    annotations: {
      readOnlyHint: true,
      destructiveHint: false,
      openWorldHint: false,
    },
  });
});

it("retains the complete accepted document alongside derived text without any host callback", async () => {
  const client = await connect();
  const result = await client.callTool({
    name: PRESENT_TOOL,
    arguments: report,
  });
  expect(result.isError).not.toBe(true);
  expect(result.structuredContent).toEqual(accepted);
  expect(result.content).toEqual([
    { type: "text", text: Presentation.accept(report).text() },
  ]);
  expect(JSON.stringify(result)).toContain("9007199254740993");
});

it.each([
  { ...report, program: 'root = Text("injected")' },
  {
    ...report,
    nodes: [{ id: "invalid", type: "Query", props: { tool: "write" } }],
  },
  { ...report, action: "save" },
  {
    ...report,
    nodes: [
      { id: "invalid", type: "Text", props: { text: "hello", onClick: "run" } },
    ],
  },
  {
    ...report,
    nodes: [
      {
        id: "invalid",
        type: "Metric",
        props: { label: "ID", value: 9007199254740992 },
      },
    ],
  },
])("refuses input outside the complete display contract: %j", async (input) => {
  const client = await connect();
  await expect(
    client.callTool({ name: PRESENT_TOOL, arguments: input }),
  ).rejects.toThrow();
  const valid = await client.callTool({
    name: PRESENT_TOOL,
    arguments: report,
  });
  expect(valid.structuredContent).toEqual(accepted);
});

it("keeps repeated presentation calls deterministic without creating a private history", async () => {
  const client = await connect();
  const first = await client.callTool({
    name: PRESENT_TOOL,
    arguments: report,
  });
  const second = await client.callTool({
    name: PRESENT_TOOL,
    arguments: report,
  });
  expect(second).toEqual(first);
  const changed = { ...report, title: "Second review" };
  const third = await client.callTool({
    name: PRESENT_TOOL,
    arguments: changed,
  });
  expect(third.structuredContent).toEqual({ ...changed, bindings: [] });
  expect(first.structuredContent).toEqual(accepted);
});

it("returns a known refusal for cross-field document violations without publishing a report", async () => {
  const client = await connect();
  const result = await client.callTool({
    name: PRESENT_TOOL,
    arguments: {
      ...report,
      title: "Invalid table",
      nodes: [
        {
          id: "invalid",
          type: "Table",
          props: {
            caption: "Deployment counts",
            columns: [{ key: "environment", label: "Environment" }],
            rowKey: "id",
            rows: [
              { id: "duplicate", environment: "Production" },
              { id: "duplicate", environment: "Staging" },
            ],
          },
        },
      ],
    },
  });
  expect(result.isError).toBe(true);
  expect(result.structuredContent).toBeUndefined();
  const valid = await client.callTool({
    name: PRESENT_TOOL,
    arguments: report,
  });
  expect(valid.structuredContent).toEqual(accepted);
});

it("publishes pure function signatures without requiring a host callback", async () => {
  const client = await connect();
  const result = await client.callTool({ name: CATALOG_TOOL, arguments: {} });
  expect(result.isError).not.toBe(true);
  expect(result.structuredContent).not.toHaveProperty("operations");
  expect(
    (result.structuredContent as { functions: unknown[] }).functions,
  ).toEqual(
    expect.arrayContaining([
      expect.objectContaining({
        name: "multiply",
        parameters: expect.any(Object),
        description: expect.any(String),
      }),
    ]),
  );
});
