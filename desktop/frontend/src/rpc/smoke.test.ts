import { afterEach, describe, expect, it } from "vitest";
import { createMemoryTransport, type MemoryTransport } from "./transports/memory";
import {
  injectRunEvent,
  injectRunFinished,
  respondSuccess,
  waitForRequest,
} from "./transports/memory.testkit";
import { createRpcClient, type RpcClient } from "./client";
import { asItemId, asRunId, asSessionId } from "./ids";
import { createMethods, type Methods } from "./methods";
import { PROTOCOL_VERSION, type Item, type RunEvent } from "@flame/runtime-contract/wire";
import discoverResponse from "@flame/runtime-contract/samples/method.discover.resp.json";

function agentMessageItem(
  id: string,
  runId: string,
  text: string,
  status: Extract<Item, { type: "agentMessage" }>["status"],
): Extract<Item, { type: "agentMessage" }> {
  return {
    id: asItemId(id),
    runId: asRunId(runId),
    status,
    createdAt: "2026-06-03T00:00:00Z",
    type: "agentMessage",
    ...(status === "running" ? {} : { phase: "finalAnswer" as const }),
    ...(text ? { content: [{ type: "text" as const, text }] } : {}),
  };
}

describe("smoke: v2 end-to-end happy path", () => {
  let transport: MemoryTransport;
  let client: RpcClient;
  let methods: Methods;

  afterEach(async () => {
    await client.close();
  });

  it("discover → create → start → interrupt → resume → completed", async () => {
    transport = createMemoryTransport();
    client = createRpcClient(transport, {
      requestMeta: () => ({
        protocolVersion: PROTOCOL_VERSION,
        clientInfo: { name: "smoke-test", version: "0.1" },
        clientCapabilities: {
          features: {},
          interruptTypes: ["approval", "question"],
        },
      }),
    });
    methods = createMethods(client);

    const discoverPromise = methods.runtime.discover();
    const discoverReq = await waitForRequest(transport, "runtime.discover");
    expect(discoverReq.params).toMatchObject({
      _meta: {
        protocolVersion: PROTOCOL_VERSION,
        clientCapabilities: { interruptTypes: ["approval", "question"] },
      },
    });
    respondSuccess(transport, discoverReq.id, discoverResponse);
    const discovery = await discoverPromise;
    expect(discovery.protocolVersion).toBe(PROTOCOL_VERSION);
    expect(discovery.serverInfo.defaultWorkspace).toEqual(
      discoverResponse.serverInfo.defaultWorkspace,
    );
    expect(discovery.capabilities.features.reasoning?.enabled).toBe(true);

    const createPromise = methods.sessions.create({ title: "smoke" });
    const createReq = await waitForRequest(transport, "sessions.create");
    expect(createReq.params).toMatchObject({
      title: "smoke",
      _meta: { protocolVersion: PROTOCOL_VERSION },
    });
    respondSuccess(transport, createReq.id, {
      id: "ses_1",
      title: "smoke",
      status: "idle",
      provider: "anthropic",
      model: "claude",
      workspace: {
        ref: { path: "/work" },
        projectRoot: "/work",
        availability: "available",
      },
      createdAt: "2026-06-03T00:00:00Z",
      updatedAt: "2026-06-03T00:00:00Z",
      revision: 1,
    });
    const session = await createPromise;
    expect(session.id).toBe("ses_1");
    expect(session.workspace.ref.path).toBe("/work");

    const startPromise = methods.runs.start({
      sessionId: asSessionId("ses_1"),
      input: [{ type: "text", text: "list files" }],
    });
    const startReq = await waitForRequest(transport, "runs.start");
    expect(startReq.params).toMatchObject({ sessionId: "ses_1" });
    respondSuccess(transport, startReq.id, {
      runId: "run_1",
      segmentId: "seg_1",
      userItemId: "item_user_1",
    });
    const { result: started, events } = await startPromise;
    expect(started.runId).toBe("run_1");
    expect(started.segmentId).toBe("seg_1");

    setTimeout(() => {
      injectRunEvent(
        transport,
        "run_1",
        "seg_1",
        "evt_1",
        {
          type: "segment.started",
          run: {
            id: asRunId("run_1"),
            sessionId: asSessionId("ses_1"),
            status: "running",
            activeSegmentId: "seg_1",
            createdAt: "2026-07-31T08:00:00.000Z",
            metrics: { steps: 0, activeDurationMillis: 0 },
            protocolProfile: { requiredFeatures: [], interruptTypes: ["approval"] },
            provider: "openai",
            model: "gpt-5",
          },
        },
        startReq.id,
      );
      injectRunEvent(
        transport,
        "run_1",
        "seg_1",
        "evt_2",
        {
          type: "item.started",
          item: agentMessageItem("item_1", "run_1", "", "running"),
        },
        startReq.id,
      );
      injectRunEvent(
        transport,
        "run_1",
        "seg_1",
        "evt_3",
        {
          type: "item.delta",
          itemId: asItemId("item_1"),
          delta: { type: "content", text: "Running ls…" },
        },
        startReq.id,
      );
      injectRunEvent(
        transport,
        "run_1",
        "seg_1",
        "evt_4",
        {
          type: "item.started",
          item: {
            id: asItemId("item_tool"),
            runId: asRunId("run_1"),
            status: "running",
            startedAt: "2026-06-03T00:00:00Z",
            type: "toolCall",
            tool: { name: "shell", arguments: { command: "ls", description: "List files" } },
          },
        },
        startReq.id,
      );
      injectRunFinished(transport, "run_1", "seg_1", "evt_5", startReq.id, {
        type: "interrupt",
        interrupts: [
          {
            itemId: asItemId("item_tool"),
            runId: "run_1" as never,
            type: "approval",
            payload: { tool: { name: "shell", arguments: { command: "ls" } }, rememberable: true },
          },
        ],
      });
    }, 0);

    const firstRun: RunEvent[] = [];
    for await (const ev of events) firstRun.push(ev);
    const finish = firstRun.at(-1)!;
    expect(finish.event.type).toBe("segment.finished");
    expect(finish.event.type === "segment.finished" && finish.event.outcome.type).toBe("interrupt");
    const interrupt =
      finish.event.type === "segment.finished" && finish.event.outcome.type === "interrupt"
        ? finish.event.outcome.interrupts[0]!
        : null;
    expect(interrupt?.itemId).toBe("item_tool");

    const resumePromise = methods.runs.resume({
      runId: asRunId("run_1"),
      responses: [
        { itemId: asItemId("item_tool"), response: { type: "approval", decision: "approve" } },
      ],
    });
    const resumeReq = await waitForRequest(transport, "runs.resume");
    expect(resumeReq.params).toMatchObject({ runId: "run_1" });
    respondSuccess(transport, resumeReq.id, { runId: "run_1", segmentId: "seg_2" });
    const { result: resumed, events: resumeEvents } = await resumePromise;
    expect(resumed.runId).toBe("run_1");
    expect(resumed.segmentId).toBe("seg_2");

    setTimeout(() => {
      injectRunEvent(
        transport,
        "run_1",
        "seg_2",
        "evt_1",
        {
          type: "segment.started",
          run: {
            id: asRunId("run_1"),
            sessionId: asSessionId("ses_1"),
            status: "running",
            activeSegmentId: "seg_1",
            createdAt: "2026-07-31T08:00:00.000Z",
            metrics: { steps: 0, activeDurationMillis: 0 },
            protocolProfile: { requiredFeatures: [], interruptTypes: ["approval"] },
            provider: "openai",
            model: "gpt-5",
          },
        },
        resumeReq.id,
      );
      injectRunEvent(
        transport,
        "run_1",
        "seg_2",
        "evt_2",
        {
          type: "item.completed",
          item: agentMessageItem("item_2", "run_1", "Found 5 files.", "completed"),
        },
        resumeReq.id,
      );
      injectRunFinished(
        transport,
        "run_1",
        "seg_2",
        "evt_3",
        resumeReq.id,
        { type: "completed" },
        { usage: { inputTokens: 100, outputTokens: 20 }, steps: 2, activeDurationMillis: 0 },
      );
    }, 0);

    const secondRun: RunEvent[] = [];
    for await (const ev of resumeEvents) secondRun.push(ev);
    expect(secondRun.map((e) => e.event.type)).toEqual([
      "segment.started",
      "item.completed",
      "segment.finished",
    ]);
  });

  it("concurrent response streams stay isolated by request identity", async () => {
    transport = createMemoryTransport();
    client = createRpcClient(transport);
    methods = createMethods(client);

    const startPromise = methods.runs.start({
      sessionId: asSessionId("ses_1"),
      input: [{ type: "text", text: "hi" }],
    });
    const req = await waitForRequest(transport, "runs.start");
    respondSuccess(transport, req.id, {
      runId: "run_ours",
      segmentId: "seg_ours",
      userItemId: "item_user_ours",
    });
    const { events } = await startPromise;

    setTimeout(() => {
      injectRunEvent(
        transport,
        "run_other",
        "seg_other",
        "evt_1",
        {
          type: "item.completed",
          item: agentMessageItem("item_x", "run_other", "stolen", "completed"),
        },
        "rpc_other",
      );
      injectRunEvent(
        transport,
        "run_ours",
        "seg_ours",
        "evt_1",
        {
          type: "item.completed",
          item: agentMessageItem("item_ok", "run_ours", "ok", "completed"),
        },
        req.id,
      );
      injectRunFinished(transport, "run_ours", "seg_ours", "evt_2", req.id);
    }, 0);

    const collected: RunEvent[] = [];
    for await (const ev of events) collected.push(ev);
    expect(collected.map((e) => e.runId)).toEqual(["run_ours", "run_ours"]);
  });

  it("malformed notification params terminate the stream at the generated boundary", async () => {
    transport = createMemoryTransport();
    client = createRpcClient(transport);
    methods = createMethods(client);

    const startPromise = methods.runs.start({
      sessionId: asSessionId("ses_1"),
      input: [{ type: "text", text: "hi" }],
    });
    const req = await waitForRequest(transport, "runs.start");
    respondSuccess(transport, req.id, {
      runId: "run_1",
      segmentId: "seg_1",
      userItemId: "item_user_1",
    });
    const { events } = await startPromise;

    setTimeout(() => {
      transport.inject(
        {
          jsonrpc: "2.0",
          method: "notifications.run.event",
          params: { runId: "run_1", event: { type: "item.started" } },
        },
        undefined,
        req.id,
      );
    }, 0);

    await expect(async () => {
      for await (const _event of events) {
      }
    }).rejects.toMatchObject({ name: "RpcProtocolError" });
  });
});
