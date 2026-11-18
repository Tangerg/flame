import { createServer } from "node:http";
import { text } from "node:stream/consumers";
import {
  createFlameClient,
  createHttpTransport,
  JSONRPC_VERSION,
  MutationLookupError,
  RpcProtocolError,
} from "@flame/runtime-client";
import discovery from "@flame/runtime-client/testing/samples/method.discover.resp.json";
import {
  RPC_HTTP,
  type ResumeRunRequest,
  type ResumeRunResponse,
  type RunAcceptance,
  type ServerCapabilities,
} from "@flame/runtime-client/wire";
import { expect, it, onTestFinished } from "vitest";

async function endpoint(reply: (method: string) => RunAcceptance | ResumeRunResponse) {
  const calls: string[] = [];
  const server = createServer(async (request, response) => {
    try {
      const { id, method } = JSON.parse(await text(request)) as { id: string; method: string };
      calls.push(method);
      const result = JSON.stringify({ jsonrpc: JSONRPC_VERSION, id, result: reply(method) });
      if (method === "runs.getAcceptance") {
        response.writeHead(RPC_HTTP.callStatus, { "Content-Type": RPC_HTTP.documentMediaType });
        response.end(result);
      } else {
        response.writeHead(RPC_HTTP.callStatus, { "Content-Type": RPC_HTTP.streamMediaType });
        response.end(`event: message\ndata: ${result}\n\n`);
      }
    } catch {
      response.writeHead(500).end();
    }
  });
  await new Promise<void>((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", resolve);
  });
  const address = server.address();
  if (!address || typeof address === "string") throw new Error("loopback address unavailable");
  const client = createFlameClient(
    createHttpTransport({ baseUrl: `http://127.0.0.1:${address.port}` }),
    { capabilities: () => discovery.capabilities as ServerCapabilities },
  );
  onTestFinished(async () => {
    try {
      await client.close();
    } finally {
      await new Promise<void>((resolve, reject) =>
        server.close((error) => (error ? reject(error) : resolve())),
      );
    }
  });
  return { client, calls };
}

const runId = "run_fixture";
const segmentId = "seg_fixture";
const userItemId = "item_opening";

it.each([
  {
    name: "omits a committed input",
    withInput: true,
    badResult: { runId, segmentId },
  },
  {
    name: "invents an input",
    withInput: false,
    badResult: { runId, segmentId, userItemId },
  },
  {
    name: "identifies a different Run",
    withInput: false,
    badResult: { runId: "run_other", segmentId },
  },
])(
  "preserves an unresolved Resume when its acknowledgement $name",
  async ({ withInput, badResult }) => {
    let receipt: RunAcceptance = { type: "notRecorded" };
    const { client, calls } = await endpoint((method) =>
      method === "runs.getAcceptance" ? receipt : badResult,
    );
    const request: ResumeRunRequest = {
      runId,
      responses: [{ itemId: "item_question", response: { type: "answer", answers: [["yes"]] } }],
      ...(withInput ? { input: [{ type: "text", text: "additional input" }] } : {}),
    };
    const opening = client.runs.resume(request);
    await expect(opening).rejects.toBeInstanceOf(RpcProtocolError);

    receipt = { ...badResult, type: "retired", sessionId: "session_fixture" };
    const [unresolved] = await client.runs.recoverCommands();
    expect(unresolved).toMatchObject({
      idempotencyKey: opening.idempotencyKey,
      operation: "resume",
      observation: { type: "unknown", cause: expect.any(MutationLookupError) },
    });
    await expect(opening.retry()).rejects.toBeInstanceOf(MutationLookupError);

    const result = { runId, segmentId, ...(withInput ? { userItemId } : {}) };
    receipt = { ...result, type: "retired", sessionId: "session_fixture" };
    await expect(opening.retry()).resolves.toEqual({
      result,
      observation: { type: "retired", sessionId: receipt.sessionId },
    });
    await expect(client.runs.recoverCommands()).resolves.toEqual([]);
    expect(calls.filter((method) => method === "runs.resume")).toHaveLength(1);
  },
);

it.each([
  { runId: "run_other", segmentId, snapshot: false },
  { runId, segmentId: "seg_other", snapshot: false },
  { runId, segmentId, snapshot: true },
])("rejects an unrelated Run subscription acknowledgement (%j)", async (received) => {
  const { client } = await endpoint(() => ({
    runId: received.runId,
    segmentId: received.segmentId,
  }));
  await expect(
    client.runs.subscribe({ runId, segmentId, snapshot: received.snapshot }),
  ).rejects.toBeInstanceOf(RpcProtocolError);
});
