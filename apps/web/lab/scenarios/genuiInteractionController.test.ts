import { unavailableComponentNotifications } from "@/test/unavailableNotifications";
import { afterEach, expect, it, vi } from "vitest";
import { type ComponentContext } from "@flame/component-sdk";
import {
  createFlameClient,
  createHttpTransport,
  type FlameClient,
  type MutationJournalStorage,
} from "@flame/runtime-client";
import { stringifyJSON } from "@frame/sdk/json";
import { type CallPluginRequest, type ServerCapabilities } from "@flame/runtime-client/wire";
import { Presentation } from "../../../../plugins/genui/contracts/document";
import {
  InteractionController,
  type InteractionState,
} from "../../../../plugins/genui/component/interactionController";
import { interfaceRequest } from "../../../../plugins/genui/contracts/fixture";
type ActionPayload = Parameters<InteractionController["execute"]>[0];

const installationId = "12345678-1234-1234-1234-123456789abc";
const contract = stringifyJSON({
  protocol: "io.github.tangerg.flame/plugin/1",
  reference: "test.records",
  composition: "selection",
  scope: "installation",
  methods: [
    { name: "records.lookup", kind: "query" },
    { name: "records.save", kind: "command" },
    { name: "records.stream", kind: "command", stream: { type: "object" } },
  ].map((method) => ({
    ...method,
    target: "installation",
    description: method.name,
    input: { type: "object" },
    output: { type: "object" },
  })),
});
const route = {
  installationId,
  digest: "1".repeat(64),
  contribution: "records",
  target: { scope: "installation", reference: installationId },
};
const releases: Array<() => Promise<void>> = [];
afterEach(async () => {
  await Promise.all(releases.splice(0).map((release) => release()));
  vi.restoreAllMocks();
});

function storage() {
  const values = new Map<string, unknown>();
  return {
    values,
    get: (key: string) => structuredClone(values.get(key)),
    set: (key: string, value: unknown) => {
      values.set(key, structuredClone(value));
    },
    remove: (key: string) => {
      values.delete(key);
    },
    keys: () => [...values.keys()],
  } satisfies MutationJournalStorage & { values: Map<string, unknown> };
}

function frame(value: unknown): string {
  return `event: message\ndata: ${JSON.stringify(value)}\n\n`;
}
function streamEvent(kind: "chunk" | "result", value: unknown): string {
  return frame({
    jsonrpc: "2.0",
    method: "notifications.plugin.stream",
    params: { kind, value: stringifyJSON(value) },
  });
}

function server() {
  const originals = new Map<string, CallPluginRequest>();
  const calls: Array<{ method: string; params: unknown; key: string | null }> = [];
  let mode: "unknown" | "success" | "failure" | "partial" = "unknown";
  let held: { arrived(): void; released: Promise<void> } | undefined;
  const fetch = vi.fn<typeof globalThis.fetch>(async (_url, init) => {
    const request = JSON.parse(init!.body as string) as {
      id: string;
      method: string;
      params: unknown;
    };
    const key = new Headers(init!.headers).get("Idempotency-Key");
    calls.push({ method: request.method, params: request.params, key });
    const success = (result: unknown) => Response.json({ jsonrpc: "2.0", id: request.id, result });
    if (request.method === "plugins.getCommand") {
      const original = originals.get((request.params as { key: string }).key);
      if (!original) throw new Error("unmodeled command receipt");
      return success(original);
    }
    if (request.method !== "plugins.call" && request.method !== "plugins.stream")
      throw new Error("unmodeled Runtime operation: " + request.method);
    if (held) {
      const original = held;
      held = undefined;
      original.arrived();
      await original.released;
    }
    const params = request.params as CallPluginRequest;
    if (params.method === "records.lookup")
      return success(
        mode === "failure"
          ? { failure: { code: "not_found", message: "owner refused" } }
          : { output: '{"amount":9007199254740993,"ratio":1.00}' },
      );
    if (!key) throw new Error("command omitted its original key");
    if (!originals.has(key)) originals.set(key, structuredClone(params));
    expect(params).toEqual(originals.get(key));
    if (mode === "unknown")
      return Response.json({
        jsonrpc: "2.0",
        id: request.id,
        error: { code: -32603, message: "reply unavailable", data: { type: "internal_error" } },
      });
    if (mode === "failure")
      return success({ failure: { code: "not_found", message: "owner refused" } });
    if (request.method === "plugins.stream") {
      const chunks =
        mode === "partial"
          ? streamEvent("chunk", { progress: 1 }) + streamEvent("chunk", { progress: 2 })
          : streamEvent("result", { saved: true });
      return new Response(frame({ jsonrpc: "2.0", id: request.id, result: {} }) + chunks, {
        headers: { "Content-Type": "text/event-stream" },
      });
    }
    return success({ output: '{"saved":true}' });
  });
  return {
    calls,
    originals,
    fetch,
    setMode(value: typeof mode) {
      mode = value;
    },
    holdNextCall() {
      if (held) throw new Error("a reply is already held");
      const arrived = Promise.withResolvers<void>();
      const released = Promise.withResolvers<void>();
      held = { arrived: arrived.resolve, released: released.promise };
      return { arrived: arrived.promise, release: () => released.resolve() };
    },
  };
}

function runtime(serverFixture: ReturnType<typeof server>, store = storage()) {
  let namespace = "idp_" + "1".repeat(32);
  const client = createFlameClient(
    createHttpTransport({ baseUrl: "https://runtime.test", fetch: serverFixture.fetch }),
    {
      mutationStorage: store,
      capabilities: () =>
        ({
          features: { plugins: { enabled: true } },
          limits: { idempotency: { namespace, retentionSeconds: 3600 } },
        }) as unknown as ServerCapabilities,
    },
  );
  releases.push(() => client.close());
  return {
    client,
    store,
    replaceStore() {
      namespace = "idp_" + "2".repeat(32);
    },
    restoreStore() {
      namespace = "idp_" + "1".repeat(32);
    },
  };
}

function presentation(method = "records.save") {
  const request = interfaceRequest();
  return Presentation.accept(
    {
      ...request,
      operations: [{ id: "save", installationId, contribution: route.contribution, method }],
    },
    [{ installationId, digest: route.digest, contribution: route.contribution, contract }],
  );
}
function action(
  name = "save",
  context: Record<string, unknown> = { amount: "9007199254740993" },
): ActionPayload {
  return {
    name,
    context,
    surfaceId: "review",
    sourceComponentId: "send",
    timestamp: "2026-11-17T00:00:00Z",
  };
}
function controller(
  client: FlameClient,
  document: Presentation,
  interaction?: ComponentContext["interaction"],
) {
  const owner = new AbortController();
  const writes: Array<{ id: string; value: unknown }> = [];
  const context: ComponentContext = {
    component: {
      installationId: "renderer-installation",
      digest: "a".repeat(64),
      componentId: "report",
    },
    runtime: client,
    interaction,
    telemetry: { read: () => ({ spans: [], metrics: [], logs: [] }), clear() {} },
    toolResult: {
      id: "original-item",
      runId: "original-run",
      name: "ui_present",
      fn: "ui_present",
      args: "{}",
      status: "ok",
      result: document.json(),
    },
    locale: "en",
    signal: owner.signal,
    track: (operation) => operation(owner.signal),
    saveFile: async () => "requested",
    notifications: unavailableComponentNotifications,
    subscribeRootRunSettlements() {
      throw new Error("unexpected settlement observation");
    },
    registerCommand() {
      throw new Error("unexpected command registration");
    },
    compose: () => null,
  };
  const state: InteractionState = {
    writeOperation: (id, value) => writes.push({ id, value }),
    resetDraft: vi.fn(),
  };
  return { value: new InteractionController(context, document, state), owner, writes };
}
async function review(value: InteractionController) {
  await vi.waitFor(() => expect(value.snapshot().action.status).toBe("review"));
  const state = value.snapshot().action;
  if (state.status !== "review") throw new Error("missing review");
  return state.review;
}
async function approve(value: InteractionController) {
  const execution = value.execute(action());
  (await review(value)).decide(true);
  await execution;
}
it("initializes recorded presentation without executing a query or command", async () => {
  const remote = server();
  const ui = controller(runtime(remote).client, presentation("records.lookup"));
  await ui.value.initialize();
  expect(remote.calls).toEqual([]);
});
it("reviews the exact captured command before sending it", async () => {
  const remote = server();
  remote.setMode("success");
  const ui = controller(runtime(remote).client, presentation());
  await ui.value.initialize();
  const payload = action();
  const execution = ui.value.execute(payload);
  const pending = await review(ui.value);
  payload.context.amount = "changed later";
  expect(remote.calls).toEqual([]);
  expect(JSON.parse(pending.input)).toEqual({ amount: "9007199254740993" });
  expect(pending.binding).toMatchObject(route);
  pending.decide(true);
  await execution;
  expect(remote.originals.size).toBe(1);
  expect(ui.value.snapshot().action.status).toBe("complete");
});
it("recovers the original unknown command without a second business intent", async () => {
  const remote = server();
  const client = runtime(remote).client;
  const ui = controller(client, presentation());
  await ui.value.initialize();
  await approve(ui.value);
  const state = ui.value.snapshot().action;
  if (state.status !== "unknown") throw new Error("missing original recovery");
  const originalKey = client.plugins.pendingCommands()[0]!.idempotencyKey;
  await ui.value.execute(action("save", { amount: "replacement" }));
  expect(remote.originals.size).toBe(1);
  remote.setMode("success");
  await ui.value.recover(state.recoveries[0]!);
  expect(ui.value.snapshot().action.status).toBe("complete");
  expect([...remote.originals.keys()]).toEqual([originalKey]);
  expect(ui.writes.at(-1)).toEqual({
    id: "save",
    value: { status: "success", data: { saved: true } },
  });
});
it("refuses new work after its mounted source is withdrawn", async () => {
  const remote = server();
  const ui = controller(runtime(remote).client, presentation());
  await ui.value.initialize();
  const execution = ui.value.execute(action());
  await review(ui.value);
  ui.owner.abort();
  await execution;
  expect(remote.calls).toEqual([]);
});
it("uses only the original followup handle and recovers its uncertain result", async () => {
  const recover = vi.fn(async () => ({
    status: "accepted" as const,
    sessionId: "original-session",
    runId: "next-run",
    userItemId: "next-item",
  }));
  const submit = vi.fn(async () => ({
    status: "unknown" as const,
    idempotencyKey: "original-key",
    recover,
  }));
  const ui = controller(runtime(server()).client, Presentation.accept(interfaceRequest(), []), {
    source: { runId: "original-run", itemId: "original-item" },
    submit,
  });
  await ui.value.initialize();
  await ui.value.execute(
    action("followup", { message: "Review", payload: { exact: "9007199254740993" } }),
  );
  expect(submit).toHaveBeenCalledExactlyOnceWith({
    action: "Review",
    payloadJSON: '{"exact":"9007199254740993"}',
  });
  const pending = ui.value.snapshot().action;
  if (pending.status !== "unknown") throw new Error("missing followup recovery");
  await ui.value.execute(action("followup", { message: "Another", payload: {} }));
  await ui.value.recover(pending.recoveries[0]!);
  expect(submit).toHaveBeenCalledOnce();
  expect(recover).toHaveBeenCalledOnce();
});
it("refuses a followup interaction belonging to another tool", async () => {
  const submit = vi.fn();
  const ui = controller(runtime(server()).client, Presentation.accept(interfaceRequest(), []), {
    source: { runId: "another-run", itemId: "another-item" },
    submit,
  });
  await ui.value.execute(action("followup", { message: "Review", payload: {} }));
  expect(submit).not.toHaveBeenCalled();
  expect(ui.value.snapshot().action.status).toBe("failed");
});
