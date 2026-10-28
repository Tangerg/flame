import { afterEach, describe, expect, it, vi } from "vitest";
import { type ComponentContext } from "@flame/component-sdk";
import {
  createFlameClient,
  createHttpTransport,
  type FlameClient,
  type MutationJournalStorage,
} from "@flame/runtime-client";
import { stringifyJSON } from "@qimu/sdk/json";
import { type CallPluginRequest, type ServerCapabilities } from "@flame/runtime-client/wire";
import {
  Presentation,
  type Action,
  type Expression,
} from "../../../plugins/genui/contracts/document";
import {
  InteractionController,
  type InteractionState,
} from "../../../plugins/genui/component/interactionController";

const installationId = "12345678-1234-1234-1234-123456789abc";
const contract = stringifyJSON({
  protocol: "io.github.tangerg.qimu/portable/1",
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

function presentation(
  options: {
    lookup?: boolean;
    stream?: boolean;
    actionId?: string;
    operationId?: string;
    steps?: Action["steps"];
  } = {},
) {
  const operationId = options.operationId ?? "save";
  const method = options.stream ? "records.stream" : "records.save";
  const operations = [
    ...(options.lookup
      ? [
          {
            id: "lookup",
            installationId,
            contribution: route.contribution,
            method: "records.lookup",
            input: { literal: {} } as Expression,
          },
        ]
      : []),
    {
      id: operationId,
      installationId,
      contribution: route.contribution,
      method,
      input: (options.lookup
        ? { operation: "lookup", path: ["data"] }
        : { object: { amount: { state: "amount" } } }) as Expression,
    },
  ];
  return Presentation.accept(
    {
      title: "Interactive records",
      state: [{ id: "amount", value: "original" }],
      nodes: [{ id: "description", type: "Text", props: { text: "Records" } }],
      actions: [
        {
          id: options.actionId ?? "submit",
          steps: options.steps ?? [
            ...(options.lookup ? [{ type: "operation" as const, operation: "lookup" }] : []),
            { type: "operation", operation: operationId },
            { type: "copy", text: { literal: "after acceptance" } },
          ],
        },
      ],
      operations,
    },
    operations.map(({ id, method: name }) => ({
      id,
      binding: { ...route, contract, method: name },
    })),
  );
}

function controller(
  client: FlameClient,
  document: Presentation,
  source = "item_original",
  componentDigest = "a".repeat(64),
) {
  const owner = new AbortController();
  const state = new Map<string, unknown>(
    document.snapshot().state.map(({ id, value }) => [id, value]),
  );
  const operations = new Map<string, unknown>();
  const writes: Array<{ id: string; value: unknown }> = [];
  const copy = vi.fn(async (_text: string) => {});
  const context: ComponentContext = {
    component: {
      installationId: "renderer-installation",
      digest: componentDigest,
      componentId: "report",
    },
    runtime: client,
    telemetry: { read: () => ({ spans: [], metrics: [], logs: [] }), clear() {} },
    toolResult: {
      id: source,
      runId: "run_original",
      name: "present_ui",
      fn: "present_ui",
      args: "{}",
      status: "ok",
      result: document.json(),
    },
    locale: "en",
    signal: owner.signal,
    track: (operation) => operation(owner.signal),
    compose: () => null,
  };
  const port: InteractionState = {
    readState: (id) => state.get(id),
    writeState: (id, value) => {
      state.set(id, value);
    },
    readOperation: (id) => operations.get(id),
    writeOperation: (id, value) => {
      operations.set(id, value);
      writes.push({ id, value });
    },
  };
  const value = new InteractionController(context, document, port, { copy, open: vi.fn() });
  return { value, state, operations, writes, copy, owner };
}

async function commandReview(value: InteractionController) {
  await vi.waitFor(() => expect(value.snapshot().status).toBe("review"));
  const review = value.snapshot();
  if (review.status !== "review") throw new Error("missing command review");
  return review.review;
}

async function approve(value: InteractionController, actionId = "submit") {
  const execution = value.execute(actionId);
  const review = await commandReview(value);
  review.decide(true);
  await execution;
  return review;
}

async function recover(value: InteractionController) {
  const current = value.snapshot();
  if (current.status !== "unknown") throw new Error("missing original recovery");
  await value.recover(current.recoveries[0]!);
}

describe("GenUI remote interaction through the public Runtime client", () => {
  it("reviews the exact numeric input and resumes the original command before continuing the action", async () => {
    const remote = server();
    const original = runtime(remote);
    const ui = controller(original.client, presentation({ lookup: true }));
    await ui.value.initialize();
    const review = await approve(ui.value);
    expect(review.binding).toEqual({ ...route, contract, method: "records.save" });
    expect(review.input).toBe('{"amount":9007199254740993,"ratio":1.00}');
    expect(ui.value.snapshot().status).toBe("unknown");
    expect(ui.copy).not.toHaveBeenCalled();
    ui.state.set("amount", "changed draft");
    ui.operations.set("lookup", { status: "success", data: { amount: "changed result" } });
    await recover(ui.value);
    expect(ui.value.snapshot().status).toBe("unknown");
    expect(ui.copy).not.toHaveBeenCalled();
    remote.setMode("success");
    await recover(ui.value);
    expect(ui.value.snapshot().status).toBe("complete");
    expect(ui.copy).toHaveBeenCalledExactlyOnceWith("after acceptance");
    expect(remote.originals.size).toBe(1);
    const commands = remote.calls.filter((call) => call.method === "plugins.call" && call.key);
    expect(new Set(commands.map((call) => call.key)).size).toBe(1);
    expect(
      commands.every((call) => (call.params as CallPluginRequest).input === review.input),
    ).toBe(true);
    expect(
      remote.calls.filter((call) => (call.params as CallPluginRequest).method === "records.lookup"),
    ).toHaveLength(1);
    expect(original.store.keys()).toEqual([]);
  });

  it("recovers after client and renderer replacement using the original binding and input", async () => {
    const remote = server();
    const original = runtime(remote);
    const document = presentation();
    const first = controller(original.client, document);
    await first.value.initialize();
    await approve(first.value);
    const firstCall = remote.calls.find((call) => call.key)!;
    first.owner.abort();
    await original.client.close();
    remote.calls.length = 0;
    const replacement = runtime(remote, original.store);
    const next = controller(
      replacement.client,
      Presentation.read(document.json()),
      "item_original",
      "b".repeat(64),
    );
    next.state.set("amount", "replacement draft");
    await next.value.initialize();
    expect(next.value.snapshot()).toMatchObject({
      status: "unknown",
      recoveries: [{ key: firstCall.key }],
    });
    expect(remote.calls).toEqual([]);
    remote.setMode("success");
    await recover(next.value);
    expect(remote.calls.map((call) => call.method)).toEqual(["plugins.getCommand", "plugins.call"]);
    expect(remote.calls[1]).toEqual(firstCall);
    expect(next.operations.get("save")).toEqual({ status: "success", data: { saved: true } });
    expect(next.copy).not.toHaveBeenCalled();
    expect(replacement.store.keys()).toEqual([]);
  });

  it("retains the original recovery after another view settles the same command", async () => {
    const remote = server();
    const current = runtime(remote);
    const ui = controller(current.client, presentation());
    await ui.value.initialize();
    await approve(ui.value);
    const original = ui.value.snapshot();
    if (original.status !== "unknown") throw new Error("missing original recovery");
    const recovery = current.client.plugins.pendingCommands()[0]!;
    const originalKey = recovery.idempotencyKey;
    remote.setMode("success");
    await expect(recovery.recover((value) => value)).resolves.toEqual({ saved: true });
    expect(current.client.plugins.pendingCommands()).toEqual([]);
    expect(ui.value.snapshot().status).toBe("unknown");

    await ui.value.recover(original.recoveries[0]!);

    expect(ui.value.snapshot().status).toBe("complete");
    expect(ui.copy).toHaveBeenCalledExactlyOnceWith("after acceptance");
    expect(remote.originals.size).toBe(1);
    expect(new Set(remote.calls.filter((call) => call.key).map((call) => call.key))).toEqual(
      new Set([originalKey]),
    );
    expect(current.store.keys()).toEqual([]);
  });

  it("does not attribute another document or action's same-binding command to this one", async () => {
    const remote = server();
    const current = runtime(remote);
    const first = controller(current.client, presentation());
    await first.value.initialize();
    await approve(first.value);
    const otherDocument = controller(current.client, presentation(), "item_other");
    await otherDocument.value.initialize();
    expect(otherDocument.value.snapshot().status).toBe("idle");
    expect(otherDocument.operations.has("save")).toBe(false);
    const otherAction = controller(current.client, presentation({ actionId: "different" }));
    await otherAction.value.initialize();
    expect(otherAction.value.snapshot().status).toBe("idle");
    expect(otherAction.operations.has("save")).toBe(false);
    expect(remote.originals.size).toBe(1);
  });

  it("keeps a lost command unknown while its original Runtime namespace is unavailable", async () => {
    const remote = server();
    const current = runtime(remote);
    const ui = controller(current.client, presentation());
    await ui.value.initialize();
    await approve(ui.value);
    current.replaceStore();
    const dispatched = remote.calls.length;
    await recover(ui.value);
    expect(ui.value.snapshot().status).toBe("unknown");
    expect(ui.value.blocked).toBe(true);
    expect(current.store.keys()).toHaveLength(1);
    expect(remote.calls).toHaveLength(dispatched);
    current.restoreStore();
    remote.setMode("success");
    await recover(ui.value);
    expect(ui.value.snapshot().status).toBe("complete");
  });

  it("treats the owner's confirmed refusal as failed and does not run continuation steps", async () => {
    const remote = server();
    remote.setMode("failure");
    const current = runtime(remote);
    const ui = controller(current.client, presentation());
    await ui.value.initialize();
    await approve(ui.value);
    expect(ui.value.snapshot()).toMatchObject({
      status: "failed",
      error: { code: "not_found", message: "owner refused" },
    });
    expect(ui.value.blocked).toBe(false);
    expect(ui.copy).not.toHaveBeenCalled();
    expect(current.store.keys()).toEqual([]);
  });

  it("keeps only the latest streaming projection and recovers the final receipt without replaying chunks", async () => {
    const remote = server();
    remote.setMode("partial");
    const current = runtime(remote);
    const document = presentation({ stream: true });
    const first = controller(current.client, document);
    await first.value.initialize();
    await approve(first.value);
    expect(first.value.snapshot().status).toBe("unknown");
    expect(
      first.writes
        .filter(({ value }) => value !== null && typeof value === "object" && "chunk" in value)
        .map(({ value }) => value),
    ).toEqual([
      { status: "loading", chunk: { progress: 1 } },
      { status: "loading", chunk: { progress: 2 } },
    ]);
    expect(first.operations.get("save")).toEqual({ status: "unknown" });
    await current.client.close();
    remote.calls.length = 0;
    remote.setMode("success");
    const nextRuntime = runtime(remote, current.store);
    const next = controller(nextRuntime.client, Presentation.read(document.json()));
    await next.value.initialize();
    await recover(next.value);
    expect(
      next.writes.some(
        ({ value }) => value !== null && typeof value === "object" && "chunk" in value,
      ),
    ).toBe(false);
    expect(next.operations.get("save")).toEqual({ status: "success", data: { saved: true } });
    expect(remote.calls.map((call) => call.method)).toEqual([
      "plugins.getCommand",
      "plugins.stream",
    ]);
    expect(nextRuntime.store.keys()).toEqual([]);
  });

  it("does not dispatch when the user declines the inspected command", async () => {
    const remote = server();
    const current = runtime(remote);
    const ui = controller(current.client, presentation());
    await ui.value.initialize();
    const execution = ui.value.execute("submit");
    await vi.waitFor(() => expect(ui.value.snapshot().status).toBe("review"));
    const review = ui.value.snapshot();
    if (review.status !== "review") throw new Error("missing command review");
    review.review.decide(false);
    await execution;
    expect(ui.value.snapshot().status).toBe("idle");
    expect(remote.calls).toEqual([]);
    expect(current.store.keys()).toEqual([]);
  });

  it("validates action-driven local state before changing a control or continuing", async () => {
    const remote = server();
    const current = runtime(remote);
    const document = Presentation.accept({
      title: "Bounded amount",
      state: [{ id: "amount", value: 5 }],
      nodes: [
        {
          id: "amount_control",
          type: "Slider",
          props: { field: "amount", label: "Amount", min: 0, max: 10 },
        },
      ],
      actions: [
        {
          id: "submit",
          steps: [
            { type: "set", state: "amount", value: { literal: 11 } },
            { type: "copy", text: { literal: "invalid state accepted" } },
          ],
        },
      ],
      operations: [],
    });
    const ui = controller(current.client, document);
    await ui.value.initialize();
    await ui.value.execute("submit");
    expect(ui.value.snapshot()).toMatchObject({
      status: "failed",
      error: { message: expect.stringContaining("slider state is outside its declared range") },
    });
    expect(ui.state.get("amount")).toBe(5);
    expect(ui.copy).not.toHaveBeenCalled();
    expect(remote.calls).toEqual([]);
  });

  it.each([
    { scenario: "an empty Each", rows: [], status: "complete", copyCount: 1 },
    { scenario: "a visible item-scoped field", rows: [true], status: "failed", copyCount: 0 },
  ])("validates the actual form controls for $scenario", async ({ rows, status, copyCount }) => {
    const remote = server();
    const current = runtime(remote);
    const document = Presentation.accept({
      title: "Conditional form",
      state: [{ id: "amount", value: "" }],
      nodes: [
        {
          id: "form",
          type: "Form",
          props: { action: "submit", submitLabel: "Submit" },
          children: [
            {
              id: "rows",
              type: "Each",
              props: { items: { literal: rows }, as: "row", limit: 1 },
              children: [
                {
                  id: "visible",
                  type: "If",
                  props: { when: { item: "row" } },
                  children: [
                    {
                      id: "amount_control",
                      type: "Input",
                      props: { field: "amount", label: "Amount", rules: { required: true } },
                    },
                  ],
                },
              ],
            },
          ],
        },
      ],
      actions: [{ id: "submit", steps: [{ type: "copy", text: { literal: "valid form" } }] }],
      operations: [],
    });
    const ui = controller(current.client, document);
    await ui.value.initialize();
    await ui.value.execute("submit");
    expect(ui.value.snapshot().status).toBe(status);
    expect(ui.copy).toHaveBeenCalledTimes(copyCount);
    expect(remote.calls).toEqual([]);
  });

  it.each([
    { operation: "query", outcome: "success" },
    { operation: "query", outcome: "failure" },
    { operation: "command", outcome: "success" },
    { operation: "command", outcome: "failure" },
    { operation: "recovery", outcome: "success" },
    { operation: "recovery", outcome: "failure" },
  ] as const)(
    "does not write a late $operation $outcome after its context retires",
    async ({ operation, outcome }) => {
      const remote = server();
      const current = runtime(remote);
      const ui = controller(current.client, presentation({ lookup: operation === "query" }));
      await ui.value.initialize();
      if (operation === "recovery") await approve(ui.value);
      remote.setMode(outcome);
      const reply = remote.holdNextCall();
      try {
        const execution = operation === "recovery" ? recover(ui.value) : ui.value.execute("submit");
        if (operation === "command") {
          const review = await commandReview(ui.value);
          review.decide(true);
        }
        await reply.arrived;
        const beforeRetirement = [...ui.writes];
        ui.owner.abort();
        const replacement = { status: "success", data: { generation: "replacement" } };
        const id = operation === "query" ? "lookup" : "save";
        ui.operations.set(id, replacement);
        reply.release();
        await execution;
        expect(ui.writes).toEqual(beforeRetirement);
        expect(ui.operations.get(id)).toBe(replacement);
        expect(ui.copy).not.toHaveBeenCalled();
      } finally {
        reply.release();
      }
    },
  );

  it("refuses local updates and resets from a retired context", async () => {
    const remote = server();
    const current = runtime(remote);
    const ui = controller(current.client, presentation());
    await ui.value.initialize();
    ui.value.update("amount", "current draft");
    ui.owner.abort();
    ui.value.update("amount", "retired draft");
    expect(ui.state.get("amount")).toBe("current draft");
    ui.value.reset();
    expect(ui.state.get("amount")).toBe("current draft");
    expect(remote.calls).toEqual([]);
  });
});
