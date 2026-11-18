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
import session from "@flame/runtime-client/testing/samples/session.json";
import { isWireStreamingMethodName, type WireMethodName } from "@flame/runtime-client/methods";
import {
  RPC_HTTP,
  type ResumeRunRequest,
  type RunAcceptance,
  type ServerCapabilities,
} from "@flame/runtime-client/wire";
import { createAgentRuntimeGateway } from "@/features/agent/adapters/agentRuntimeGateway";
import { createAgentCommands } from "@/features/agent/application/agentCommandOwner";
import { createAgentSessionUsage } from "@/features/agent/application/session/sessionUsage";
import { createAgentSharedMaterial } from "@/features/agent/application/session/sharedMaterial";
import { expect, it, onTestFinished } from "vitest";

async function endpoint(
  reply: (method: WireMethodName, params: Record<string, unknown>) => unknown,
) {
  const calls: WireMethodName[] = [];
  const server = createServer(async (request, response) => {
    try {
      const { id, method, params } = JSON.parse(await text(request)) as {
        id: string;
        method: WireMethodName;
        params: Record<string, unknown>;
      };
      calls.push(method);
      const result = JSON.stringify({
        jsonrpc: JSONRPC_VERSION,
        id,
        result: reply(method, params),
      });
      if (isWireStreamingMethodName(method)) {
        response.writeHead(RPC_HTTP.callStatus, { "Content-Type": RPC_HTTP.streamMediaType });
        response.end(`event: message\ndata: ${result}\n\n`);
      } else {
        response.writeHead(RPC_HTTP.callStatus, { "Content-Type": RPC_HTTP.documentMediaType });
        response.end(result);
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

it.each(["ses_other", "ses_source"])(
  "retains a recovered input through passive Session reads of %s",
  async (firstRead) => {
    const sessionId = "ses_source";
    let accepted = false;
    const { client, calls } = await endpoint((method, params) => {
      switch (method) {
        case "sessions.snapshot":
          return {
            session: { ...session, id: params.sessionId },
            items: [],
            runs: [],
            interrupts: [],
          };
        case "runs.getAcceptance":
          return accepted
            ? { type: "retired", sessionId, runId, segmentId, userItemId }
            : { type: "notRecorded" };
        case "runs.start":
          accepted = true;
          return { runId, segmentId };
        default:
          throw new Error(`unexpected fixture method ${method}`);
      }
    });
    const { gateway, dispose } = createAgentRuntimeGateway(
      () => client,
      { hasCapability: () => false },
      createAgentSharedMaterial(),
      createAgentCommands(),
      createAgentSessionUsage(),
    );
    try {
      const opening = client.runs.start({
        sessionId,
        input: [{ type: "text", text: "captured input" }],
      });
      await expect(opening).rejects.toBeInstanceOf(RpcProtocolError);
      const passive = await gateway.loadSessionSnapshot(firstRead);
      expect(passive).not.toBeNull();
      if (firstRead !== sessionId) {
        expect(passive?.recoveredInputs).toBeUndefined();
        expect(passive?.acknowledgeRecoveredCommands).toBeUndefined();
      }
      const material = await gateway.loadSessionSnapshot(sessionId);
      expect(material?.recoveredInputs).toEqual([
        expect.objectContaining({
          idempotencyKey: opening.idempotencyKey,
          acceptance: { status: "retired", sessionId, runId, userItemId },
        }),
      ]);
      if (firstRead === sessionId) material?.recoveredInputs?.[0]?.acknowledge();
      else material?.acknowledgeRecoveredCommands?.();
      await expect(client.runs.recoverCommands()).resolves.toEqual([]);
      expect(calls.filter((method) => method === "runs.start")).toHaveLength(1);
    } finally {
      dispose();
    }
  },
);
