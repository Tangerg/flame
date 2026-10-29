import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, onTestFinished, vi } from "vitest";
import type { ComponentContext } from "@flame/component-sdk";
import {
  createFlameClient,
  createHttpTransport,
  type MutationJournalStorage,
} from "@flame/runtime-client";
import { stringifyJSON } from "@qimu/sdk/json";
import type { CallPluginRequest, ServerCapabilities } from "@flame/runtime-client/wire";
import { createTestClient } from "@/test/renderClient";
import { mockSelectLayout } from "@/test/selectLayout";
import { Computer } from "../../../../plugins/computer/component/component";
import { METHODS } from "../../../../plugins/computer/contracts/methods";
import { LIMITS } from "../../../../plugins/computer/contracts/limits";
import { CAPABILITY_JSON } from "../../../../plugins/computer/component/methods.generated";
import type {
  Approval,
  OperationRecord,
  Snapshot,
  Surface,
} from "../../../../plugins/computer/component/types";

const installationId = "12345678-1234-4234-9234-123456789abc";
const digest = "a".repeat(64);
const initial: Snapshot = {
  configured: true,
  host: { hostname: "isolated-runtime", platform: "linux", arch: "x64" },
  session: {
    id: "control-1",
    workerId: "worker-1",
    state: "ready",
    surfaceDigest: digest,
    openedAt: "2026-10-28T00:00:00.000Z",
    expiresAt: "2026-10-28T00:15:00.000Z",
  },
};
const png =
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQIHWP4z8DwHwAFgAI/ScLbtAAAAABJRU5ErkJggg==";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function frame(value: unknown) {
  return `event: message\ndata: ${JSON.stringify(value)}\n\n`;
}
function chunk(value: unknown) {
  return frame({
    jsonrpc: "2.0",
    method: "notifications.plugin.stream",
    params: { kind: "chunk", value: stringifyJSON(value) },
  });
}

function fixture(value = initial) {
  let snapshot = structuredClone(value);
  const subscribers = new Set<ReadableStreamDefaultController<Uint8Array>>();
  const encoder = new TextEncoder();
  const calls: Array<{ method: string; params: CallPluginRequest; key: string | null }> = [];
  const originals = new Map<string, CallPluginRequest>();
  const outcomes = new Map<string, unknown>();
  const records = new Map<string, OperationRecord>();
  let executeMode: "success" | "unknown" | "held" = "success";
  let blocked: { resolve(value: Response): void; id: string | number; key: string } | undefined;
  let unavailableLookup = false;
  let unavailableContext = false;
  let instructions = "Use the isolated fixture controls only.";
  const unknownLifecycleMethods = new Set<string>();
  const unknownLifecycleCommands = new Set<string>();
  let heldContext: { arrived: Promise<void>; release(): void } | undefined;
  let contextGate: Promise<void> | undefined;
  let contextArrived: (() => void) | undefined;
  let replyGate: Promise<void> | undefined;
  let resultGate: { promise: Promise<void>; arrived(): void } | undefined;
  let unavailableResult = false;
  let historyPageSize: number | undefined;

  function publish(next: Snapshot) {
    snapshot = structuredClone(next);
    for (const subscriber of subscribers) subscriber.enqueue(encoder.encode(chunk(snapshot)));
  }
  function context(): Surface {
    if (!snapshot.session) throw new Error("fixture has no live session");
    return {
      sessionId: snapshot.session.id,
      workerId: snapshot.session.workerId,
      surfaceDigest: snapshot.session.surfaceDigest!,
      expiresAt: snapshot.session.expiresAt,
      host: snapshot.host,
      instructions,
      toolsJSON: JSON.stringify([
        {
          name: "js",
          inputSchema: {
            type: "object",
            properties: { code: { type: "string" } },
            required: ["code"],
          },
        },
        { name: "js_reset", inputSchema: { type: "object", properties: {} } },
      ]),
      serverJSON: '{"name":"fixture-computer","version":"1"}',
    };
  }
  const fetch = vi.fn<typeof globalThis.fetch>(async (_url, init) => {
    const request = JSON.parse(init!.body as string) as {
      id: string | number;
      method: string;
      params: CallPluginRequest;
    };
    const params = request.params;
    const key = new Headers(init!.headers).get("Idempotency-Key");
    calls.push({ method: request.method, params, key });
    const success = (result: unknown) => Response.json({ jsonrpc: "2.0", id: request.id, result });
    const output = (result: unknown) => success({ output: stringifyJSON(result) });
    const unknown = () =>
      Response.json({
        jsonrpc: "2.0",
        id: request.id,
        error: {
          code: -32603,
          message: "fixture acknowledgement lost",
          data: { type: "internal_error" },
        },
      });
    if (request.method === "plugins.getCommand") {
      if (unavailableLookup) return unknown();
      const original = originals.get((params as unknown as { key: string }).key);
      if (!original) throw new Error("fixture original command is missing");
      return success(original);
    }
    if (request.method === "plugins.stream") {
      if (params.method !== METHODS.watch)
        throw new Error("fixture only streams control snapshots");
      let controller: ReadableStreamDefaultController<Uint8Array>;
      const stop = () => {
        if (!subscribers.delete(controller)) return;
        controller.close();
      };
      const body = new ReadableStream<Uint8Array>({
        start(value) {
          controller = value;
          subscribers.add(value);
          value.enqueue(
            encoder.encode(frame({ jsonrpc: "2.0", id: request.id, result: {} }) + chunk(snapshot)),
          );
          init?.signal?.addEventListener("abort", stop, { once: true });
        },
        cancel() {
          subscribers.delete(controller);
          init?.signal?.removeEventListener("abort", stop);
        },
      });
      return new Response(body, { headers: { "Content-Type": "text/event-stream" } });
    }
    if (request.method !== "plugins.call")
      throw new Error("unexpected fixture operation: " + request.method);
    if (params.method === METHODS.read) return output(snapshot);
    if (params.method === METHODS.context) {
      if (unavailableContext) return unknown();
      const captured = context();
      if (contextGate) {
        const gate = contextGate;
        contextGate = undefined;
        contextArrived?.();
        await gate;
      }
      return output(captured);
    }
    if (params.method === METHODS.history) {
      const start = Number((JSON.parse(params.input) as { before?: string }).before ?? 0);
      const entries = [...records.values()];
      const end = start + (historyPageSize ?? entries.length);
      return output({
        entries: entries
          .slice(start, end)
          .map(
            ({ inputJSON: _input, invocationJSON: _invocation, outcomeJSON: _outcome, ...entry }) =>
              entry,
          ),
        ...(end < entries.length ? { next: String(end) } : {}),
      });
    }
    if (params.method === METHODS.result) {
      const record = structuredClone(records.get((JSON.parse(params.input) as { id: string }).id));
      const gate = resultGate;
      resultGate = undefined;
      if (gate) {
        gate.arrived();
        await gate.promise;
      }
      if (unavailableResult) return unknown();
      if (!record) throw new Error("fixture operation record is missing");
      return output(record);
    }
    if (!key) throw new Error("fixture command has no original identity");
    const input = JSON.parse(params.input) as Record<string, unknown>;
    if (originals.has(key)) {
      if (JSON.stringify(params) !== JSON.stringify(originals.get(key)))
        throw new Error("fixture command replay changed its captured input");
      if (unknownLifecycleCommands.has(key)) return unknown();
      if (executeMode === "unknown" && params.method === METHODS.execute) return unknown();
      return output(outcomes.get(key));
    }
    originals.set(key, structuredClone(params));
    records.set(key, {
      id: key,
      kind: "command",
      method: params.method,
      admittedAt: "2026-10-28T00:01:00.000Z",
      dispatched: true,
      settled: false,
      inputJSON: params.input,
      invocationJSON: '{"origin":"human"}',
    });
    let result: unknown;
    if (params.method === METHODS.open) {
      publish({ ...snapshot, session: structuredClone(initial.session) });
      result = snapshot.session;
    } else if (params.method === METHODS.close || params.method === METHODS.stop) {
      if (input.sessionId !== snapshot.session?.id)
        throw new Error("fixture command changed its captured session");
      if (params.method === METHODS.stop && input.operationId !== snapshot.session?.operation?.id)
        throw new Error("fixture stop changed its original operation");
      publish({
        ...snapshot,
        session: {
          ...snapshot.session!,
          state: "closed",
          reason: "user",
          operation: undefined,
          approval: undefined,
        },
      });
      result = snapshot.session;
      if (blocked) {
        blocked.resolve(
          Response.json({
            jsonrpc: "2.0",
            id: blocked.id,
            error: {
              code: -32603,
              message: "fixture cancellation has no original outcome",
              data: { type: "internal_error" },
            },
          }),
        );
        blocked = undefined;
      }
    } else if (params.method === METHODS.reply) {
      await replyGate;
      if (input.requestId !== snapshot.session?.approval?.id)
        return success({
          failure: { code: "permission_retired", message: "the original permission request ended" },
        });
      publish({ ...snapshot, session: { ...snapshot.session!, approval: undefined } });
      result = true;
    } else if (params.method === METHODS.execute) {
      result = {
        commandId: key,
        result: { content: [{ type: "text", text: "Original fixture result." }] },
        cleanup: "ended",
      };
      outcomes.set(key, result);
      if (executeMode === "unknown") return unknown();
      if (executeMode === "held") {
        publish({
          ...snapshot,
          session: {
            ...snapshot.session!,
            state: "executing",
            operation: { id: key, tool: "js", startedAt: "2026-10-28T00:01:00.000Z" },
          },
        });
        return await new Promise<Response>((resolve, reject) => {
          blocked = { resolve, id: request.id, key };
          init?.signal?.addEventListener("abort", () => reject(init.signal!.reason), {
            once: true,
          });
        });
      }
    } else throw new Error("unexpected fixture command: " + params.method);
    if (unknownLifecycleMethods.has(params.method)) {
      unknownLifecycleCommands.add(key);
      return unknown();
    }
    outcomes.set(key, result);
    records.set(key, {
      ...records.get(key)!,
      settled: true,
      outcomeJSON: stringifyJSON({ value: result }),
    });
    return output(result);
  });
  return {
    fetch,
    calls,
    originals,
    records,
    snapshot: () => structuredClone(snapshot),
    publish,
    setExecuteMode(value: typeof executeMode) {
      executeMode = value;
    },
    finishExecution() {
      if (!blocked) throw new Error("fixture execution is not pending");
      const original = blocked;
      blocked = undefined;
      const result = outcomes.get(original.key);
      if (result === undefined) throw new Error("fixture execution outcome is missing");
      records.set(original.key, {
        ...records.get(original.key)!,
        settled: true,
        outcomeJSON: stringifyJSON({ value: result }),
      });
      publish({
        ...snapshot,
        session: { ...snapshot.session!, state: "ready", operation: undefined },
      });
      original.resolve(
        Response.json({
          jsonrpc: "2.0",
          id: original.id,
          result: { output: stringifyJSON(result) },
        }),
      );
    },
    setLookupUnavailable(value: boolean) {
      unavailableLookup = value;
    },
    setContextUnavailable(value: boolean) {
      unavailableContext = value;
    },
    setInstructions(value: string) {
      instructions = value;
    },
    setLifecycleUnknown(method: string, value: boolean) {
      if (value) unknownLifecycleMethods.add(method);
      else unknownLifecycleMethods.delete(method);
    },
    endWatch() {
      for (const subscriber of subscribers) subscriber.close();
      subscribers.clear();
    },
    holdContext() {
      const gate = Promise.withResolvers<void>();
      const arrived = Promise.withResolvers<void>();
      contextGate = gate.promise;
      contextArrived = arrived.resolve;
      heldContext = { arrived: arrived.promise, release: gate.resolve };
      return heldContext;
    },
    holdReply() {
      const gate = Promise.withResolvers<void>();
      replyGate = gate.promise;
      return gate.resolve;
    },
    holdResult() {
      const gate = Promise.withResolvers<void>();
      const arrived = Promise.withResolvers<void>();
      resultGate = { promise: gate.promise, arrived: arrived.resolve };
      return { arrived: arrived.promise, release: gate.resolve };
    },
    setResultUnavailable(value: boolean) {
      unavailableResult = value;
    },
    setHistoryPageSize(value: number) {
      historyPageSize = value;
    },
  };
}

function rig(server = fixture()) {
  const journal = new Map<string, unknown>();
  const storage: MutationJournalStorage = {
    get: (key) => structuredClone(journal.get(key)),
    set: (key, value) => {
      journal.set(key, structuredClone(value));
    },
    remove: (key) => {
      journal.delete(key);
    },
    keys: () => [...journal.keys()],
  };
  const client = createFlameClient(
    createHttpTransport({ baseUrl: "https://runtime.test", fetch: server.fetch }),
    {
      mutationStorage: storage,
      capabilities: () =>
        ({
          features: { plugins: { enabled: true } },
          limits: { idempotency: { namespace: "idp_" + "1".repeat(32), retentionSeconds: 3600 } },
        }) as unknown as ServerCapabilities,
    },
  );
  const ui = createTestClient({ start: false, runtimeClient: () => client });
  const controllers: AbortController[] = [];
  onTestFinished(async () => {
    controllers.forEach((controller) => controller.abort());
    await client.close();
    ui.dispose();
  });
  return {
    server,
    client,
    mount(locale = "en") {
      const controller = new AbortController();
      controllers.push(controller);
      const context: ComponentContext = {
        component: { installationId, digest, componentId: "computer" },
        binding: {
          installationId,
          digest,
          contribution: "computer-control",
          contract: CAPABILITY_JSON,
        },
        locale,
        runtime: client,
        signal: controller.signal,
        telemetry: { read: () => ({ spans: [], metrics: [], logs: [] }), clear: () => undefined },
        track: (operation) => operation(controller.signal),
        saveFile: async () => "requested",
        compose: () => {
          throw new Error("computer console must not compose other plugins");
        },
      };
      return { ...ui.render(<Computer context={context} />), controller };
    },
  };
}

async function review(code = "await fixture.perform();") {
  const editor = await screen.findByRole("textbox", { name: "Tool arguments (JSON)" });
  fireEvent.change(editor, { target: { value: JSON.stringify({ code }) } });
  fireEvent.click(screen.getByRole("button", { name: "Review operation" }));
  return screen.findByRole("button", { name: "Execute reviewed operation" });
}

it.each(["[]", "null", "{"])(
  "identifies invalid arguments %s at the editable field and retains the corrected draft",
  async (value) => {
    const setup = rig();
    setup.mount();
    const editor = await screen.findByRole("textbox", { name: "Tool arguments (JSON)" });
    fireEvent.change(editor, { target: { value } });
    const prepare = screen.getByRole("button", { name: "Review operation" });
    act(() => prepare.focus());
    fireEvent.click(prepare);
    const failure = await screen.findByRole("alert");
    expect(editor.getAttribute("aria-invalid")).toBe("true");
    expect(editor.getAttribute("aria-describedby")).toBe(failure.id);
    expect(document.activeElement).toBe(editor);
    expect(setup.server.originals.size).toBe(0);

    const draft = JSON.stringify({ code: "await fixture.perform();" });
    fireEvent.change(editor, { target: { value: draft } });
    expect(screen.queryByRole("alert")).toBeNull();
    expect(editor.getAttribute("aria-invalid")).not.toBe("true");
    expect(editor.getAttribute("aria-describedby")).toBeNull();
    fireEvent.click(prepare);
    const back = await screen.findByRole("button", { name: "Back to editor" });
    expect(document.activeElement).toBe(back);
    fireEvent.click(back);
    const restored = (await screen.findByRole("textbox", {
      name: "Tool arguments (JSON)",
    })) as HTMLTextAreaElement;
    await waitFor(() => expect(document.activeElement).toBe(restored));
    expect(restored.value).toBe(draft);
    expect(setup.server.originals.size).toBe(0);
  },
);

it("does not reclaim later console focus after returning from review", async () => {
  const setup = rig();
  setup.mount();
  await review();
  fireEvent.click(screen.getByRole("button", { name: "Back to editor" }));
  const outside = screen.getByRole("button", { name: "End control lease" });
  act(() => outside.focus());
  await act(() => new Promise<void>((resolve) => requestAnimationFrame(() => resolve())));
  expect(document.activeElement).toBe(outside);
  expect(setup.server.originals.size).toBe(0);
});

it.each(["editor", "later control"])(
  "keeps the arguments readable during execution and preserves %s focus on settlement",
  async (destination) => {
    const setup = rig();
    setup.server.setExecuteMode("held");
    setup.mount();
    const execute = await review();
    act(() => execute.focus());
    fireEvent.click(execute);
    await screen.findByRole("button", { name: "Stop current operation" });
    const editor = screen.getByRole("textbox", {
      name: "Tool arguments (JSON)",
    }) as HTMLTextAreaElement;
    await waitFor(() => expect(document.activeElement).toBe(editor));
    expect(editor.readOnly).toBe(true);
    expect(editor.disabled).toBe(false);
    expect(editor.value).toBe(JSON.stringify({ code: "await fixture.perform();" }));
    expect(screen.getByRole("button", { name: "Review operation" }).hasAttribute("disabled")).toBe(
      true,
    );
    expect(setup.server.originals.size).toBe(1);

    const target =
      destination === "editor" ? editor : screen.getByRole("button", { name: "End control lease" });
    act(() => {
      target.focus();
      setup.server.finishExecution();
    });
    await waitFor(() => expect(editor.readOnly).toBe(false));
    expect(document.activeElement).toBe(target);
    expect(editor.value).toBe(JSON.stringify({ code: "await fixture.perform();" }));
    expect(setup.server.originals.size).toBe(1);
  },
);

it("retains focus and readable arguments when the original execution outcome is unknown", async () => {
  const setup = rig();
  setup.server.setExecuteMode("unknown");
  setup.mount();
  const execute = await review();
  act(() => execute.focus());
  fireEvent.click(execute);
  await screen.findByRole("button", { name: "Check original outcome" });
  const editor = screen.getByRole("textbox", {
    name: "Tool arguments (JSON)",
  }) as HTMLTextAreaElement;
  await waitFor(() => expect(document.activeElement).toBe(editor));
  expect(editor.readOnly).toBe(true);
  expect(editor.disabled).toBe(false);
  expect(setup.server.originals.size).toBe(1);
});

function nativeApproval(id = "permission-1", persist = ["session"]): Approval {
  return {
    id,
    sessionId: "control-1",
    operationId: "operation-1",
    requestJSON: JSON.stringify({
      mode: "form",
      message: "Allow control of the test editor?",
      requestedSchema: { type: "object", properties: {} },
      _meta: {
        codex_approval_kind: "mcp_tool_call",
        connector_id: "computer-use",
        tool_params: { app: "com.example.editor" },
        persist,
        riskLevel: "high",
        warningSubtitle: "This editor can modify files.",
      },
    }),
  };
}

it("opens an explicit bounded lease on the displayed Runtime host only after a human action", async () => {
  const setup = rig(fixture({ ...initial, session: undefined }));
  setup.mount();
  expect(await screen.findByText("isolated-runtime")).not.toBeNull();
  expect(screen.getByText(/operator console outside the controlled desktop/)).not.toBeNull();
  expect(setup.server.originals.size).toBe(0);
  const lease = screen.getByRole("textbox", { name: "Lease duration in seconds" });
  expect((lease as HTMLInputElement).value).toBe(String(LIMITS.defaultLeaseSeconds));
  for (const value of [
    LIMITS.minLeaseSeconds - 1,
    LIMITS.maxLeaseSeconds + 1,
    LIMITS.minLeaseSeconds + 0.5,
  ]) {
    fireEvent.change(lease, { target: { value: String(value) } });
    fireEvent.click(screen.getByRole("button", { name: "Authorize and open control" }));
    expect((await screen.findByRole("alert")).textContent).toContain(
      `${LIMITS.minLeaseSeconds} to ${LIMITS.maxLeaseSeconds}`,
    );
    expect(setup.server.originals.size).toBe(0);
  }
  fireEvent.change(lease, { target: { value: String(LIMITS.minLeaseSeconds) } });
  fireEvent.click(screen.getByRole("button", { name: "Authorize and open control" }));
  await screen.findByRole("button", { name: "End control lease" });
  const [original] = setup.server.originals.values();
  expect(original!.method).toBe(METHODS.open);
  expect(JSON.parse(original!.input)).toEqual({ leaseSeconds: LIMITS.minLeaseSeconds });
});

it("displays original native risks and offers only upstream persistence choices", async () => {
  const approval = nativeApproval();
  const setup = rig(fixture({ ...initial, session: { ...initial.session!, approval } }));
  setup.mount();
  await screen.findByText("Allow control of the test editor?");
  expect(screen.getByText("com.example.editor")).not.toBeNull();
  expect(screen.getByText("This editor can modify files.")).not.toBeNull();
  expect(screen.queryByRole("button", { name: "Always allow" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Allow for this session" }));
  await waitFor(() => expect(setup.server.originals.size).toBe(1));
  const [original] = setup.server.originals.values();
  expect(JSON.parse(original!.input)).toEqual({
    sessionId: "control-1",
    requestId: "permission-1",
    responseJSON: '{"action":"accept","content":{},"_meta":{"persist":"session"}}',
  });
  await waitFor(() => expect(screen.queryByText("Allow control of the test editor?")).toBeNull());
});

it("keeps a cancelled original approval distinct from a replacement with the same message", async () => {
  const setup = rig(
    fixture({ ...initial, session: { ...initial.session!, approval: nativeApproval() } }),
  );
  setup.mount();
  await screen.findByText("Allow control of the test editor?");
  act(() =>
    setup.server.publish({
      ...initial,
      session: { ...initial.session!, approval: nativeApproval("permission-2", []) },
    }),
  );
  await screen.findByText("permission-2");
  expect(screen.queryByRole("button", { name: "Allow for this session" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Decline" }));
  await waitFor(() => expect(setup.server.originals.size).toBe(1));
  const input = JSON.parse([...setup.server.originals.values()][0]!.input) as {
    requestId: string;
    responseJSON: string;
  };
  expect(input.requestId).toBe("permission-2");
  expect(JSON.parse(input.responseJSON)).toEqual({ action: "decline" });
});

it.each(["editor", "outside action"])(
  "preserves permission reply focus until retirement and returns to %s",
  async (destination) => {
    const server = fixture({
      ...initial,
      session: { ...initial.session!, approval: nativeApproval() },
    });
    const release = server.holdReply();
    onTestFinished(() => release());
    const setup = rig(server);
    setup.mount();
    const editor = await screen.findByRole<HTMLTextAreaElement>("textbox", {
      name: "Tool arguments (JSON)",
    });
    const decline = screen.getByRole<HTMLButtonElement>("button", { name: "Decline" });
    decline.focus();
    fireEvent.click(decline);
    await waitFor(() => expect(decline.getAttribute("aria-disabled")).toBe("true"));
    expect(decline.disabled).toBe(false);
    expect(document.activeElement).toBe(decline);
    fireEvent.click(decline);
    const outside = screen.getByRole<HTMLButtonElement>("button", { name: "Refresh outcomes" });
    if (destination === "outside action") outside.focus();
    await act(async () => release());
    await waitFor(() => expect(screen.queryByRole("button", { name: "Decline" })).toBeNull());
    expect(document.activeElement).toBe(destination === "editor" ? editor : outside);
    expect(
      [...server.originals.values()].filter((item) => item.method === METHODS.reply),
    ).toHaveLength(1);
  },
);

it("returns a retired permission to status controls when the captured editor is unavailable", async () => {
  const server = fixture({
    ...initial,
    session: { ...initial.session!, approval: nativeApproval() },
  });
  server.setContextUnavailable(true);
  const setup = rig(server);
  setup.mount();
  const decline = await screen.findByRole<HTMLButtonElement>("button", { name: "Decline" });
  await screen.findByRole("alert");
  expect(screen.queryByRole("textbox", { name: "Tool arguments (JSON)" })).toBeNull();
  decline.focus();
  fireEvent.click(decline);
  await waitFor(() => expect(screen.queryByRole("button", { name: "Decline" })).toBeNull());
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "Refresh status" }));
});

it("validates requested form fields and preserves exact numeric input until its owner admits it", async () => {
  mockSelectLayout();
  const approval: Approval = {
    id: "form-1",
    sessionId: "control-1",
    operationId: "operation-1",
    requestJSON: JSON.stringify({
      mode: "form",
      message: "Provide the requested delivery information.",
      requestedSchema: {
        type: "object",
        properties: {
          name: { type: "string", title: "Recipient", minLength: 2 },
          count: { type: "integer", title: "Count", minimum: 1 },
          confirmed: { type: "boolean", title: "Confirmed", default: false },
          channel: {
            type: "string",
            title: "Channel",
            oneOf: [
              { const: "mail", title: "Email" },
              { const: "chat", title: "Chat" },
            ],
          },
          options: {
            type: "array",
            title: "Options",
            items: {
              anyOf: [
                { const: "first", title: "First option" },
                { const: "second", title: "Second option" },
              ],
            },
            minItems: 1,
          },
        },
        required: ["name", "count", "channel", "options"],
      },
    }),
  };
  const setup = rig(fixture({ ...initial, session: { ...initial.session!, approval } }));
  setup.mount();
  await screen.findByText("Provide the requested delivery information.");
  fireEvent.click(screen.getByRole("button", { name: "Submit approval" }));
  await screen.findByRole("alert");
  expect(setup.server.originals.size).toBe(0);
  fireEvent.change(screen.getByRole("textbox", { name: "Recipient (required)" }), {
    target: { value: "Operator" },
  });
  fireEvent.change(screen.getByRole("textbox", { name: "Count (required)" }), {
    target: { value: "9007199254740993" },
  });
  fireEvent.click(screen.getByRole("combobox", { name: "Channel (required)" }));
  const channel = await screen.findByRole("option", { name: "Email" });
  fireEvent.pointerDown(channel, { pointerType: "mouse" });
  fireEvent.click(channel);
  await waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
  expect(screen.getByRole("combobox", { name: "Channel (required)" }).textContent).toContain(
    "Email",
  );
  fireEvent.click(screen.getByLabelText("Second option", { selector: "input" }));
  expect(screen.getByRole("checkbox", { name: "Second option" }).getAttribute("aria-checked")).toBe(
    "true",
  );
  fireEvent.click(screen.getByRole("button", { name: "Submit approval" }));
  await waitFor(() => expect(screen.getByRole("alert").textContent).toContain("permission"));
  expect(setup.server.originals.size).toBe(0);
  fireEvent.change(screen.getByRole("textbox", { name: "Count (required)" }), {
    target: { value: "3" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Submit approval" }));
  await waitFor(() => expect(setup.server.originals.size).toBe(1));
  const input = JSON.parse([...setup.server.originals.values()][0]!.input) as {
    requestId: string;
    responseJSON: string;
  };
  expect(JSON.parse(input.responseJSON)).toEqual({
    action: "accept",
    content: { confirmed: false, name: "Operator", count: 3, channel: "mail", options: ["second"] },
  });
});

it("requires a separate explicit reply after the requested external URL step", async () => {
  const approval: Approval = {
    id: "url-1",
    sessionId: "control-1",
    operationId: "operation-1",
    requestJSON:
      '{"mode":"url","message":"Finish the external account check.","url":"https://permission.example/step","elicitationId":"external-1"}',
  };
  const setup = rig(fixture({ ...initial, session: { ...initial.session!, approval } }));
  setup.mount();
  const link = await screen.findByRole("link", { name: "https://permission.example/step" });
  expect(link.getAttribute("rel")).toBe("noopener noreferrer");
  expect(setup.server.originals.size).toBe(0);
  fireEvent.click(screen.getByRole("button", { name: "Submit approval" }));
  await waitFor(() => expect(setup.server.originals.size).toBe(1));
  const input = JSON.parse([...setup.server.originals.values()][0]!.input) as {
    requestId: string;
    responseJSON: string;
  };
  expect(JSON.parse(input.responseJSON)).toEqual({ action: "accept" });
});

it("recovers the original unknown command after remount without another command identity", async () => {
  const setup = rig();
  setup.server.setExecuteMode("unknown");
  const first = setup.mount();
  fireEvent.click(await review());
  await screen.findByRole("button", { name: "Check original outcome" });
  expect(setup.server.originals.size).toBe(1);
  const [originalKey, originalInput] = [...setup.server.originals.entries()][0]!;
  first.unmount();
  first.controller.abort();
  setup.mount();
  await screen.findByRole("button", { name: "Check original outcome" });
  expect(
    (await screen.findByRole("textbox", { name: "Tool arguments (JSON)" })).hasAttribute(
      "readonly",
    ),
  ).toBe(true);
  expect(screen.getByRole("button", { name: "Review operation" }).hasAttribute("disabled")).toBe(
    true,
  );
  setup.server.setLookupUnavailable(true);
  fireEvent.click(screen.getByRole("button", { name: "Check original outcome" }));
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "Check original outcome" })).not.toBeNull(),
  );
  expect(setup.server.originals.size).toBe(1);
  setup.server.setLookupUnavailable(false);
  setup.server.setExecuteMode("success");
  fireEvent.click(screen.getByRole("button", { name: "Check original outcome" }));
  await waitFor(() =>
    expect(screen.queryByRole("button", { name: "Check original outcome" })).toBeNull(),
  );
  expect(setup.server.originals.size).toBe(1);
  expect(
    setup.server.calls
      .filter((call) => call.params.method === METHODS.execute)
      .every(
        (call) =>
          call.key === originalKey && JSON.stringify(call.params) === JSON.stringify(originalInput),
      ),
  ).toBe(true);
  expect(setup.server.calls.some((call) => call.method === "plugins.getCommand")).toBe(true);
});

it("keeps stop available during an outstanding execution and preserves its unknown outcome", async () => {
  const setup = rig();
  setup.server.setExecuteMode("held");
  setup.mount();
  fireEvent.click(await review());
  const stop = await screen.findByRole("button", { name: "Stop current operation" });
  expect(stop.hasAttribute("disabled")).toBe(false);
  expect(screen.getByRole("button", { name: "End control lease" }).hasAttribute("disabled")).toBe(
    false,
  );
  fireEvent.click(stop);
  await screen.findByRole("button", { name: "Check original outcome" });
  expect(setup.server.originals.size).toBe(2);
  const [execution, cancellation] = [...setup.server.originals.entries()];
  expect(cancellation![1].method).toBe(METHODS.stop);
  expect(JSON.parse(cancellation![1].input)).toEqual({
    sessionId: "control-1",
    operationId: execution![0],
  });
  expect(await screen.findByText("Closed", { exact: true })).not.toBeNull();
});

it.each([
  { method: METHODS.close, action: "End control lease" },
  { method: METHODS.stop, action: "Stop current operation" },
])(
  "keeps $action available for a later lease while its original command remains unknown",
  async ({ method, action }) => {
    const setup = rig(
      fixture({
        ...initial,
        session: {
          ...initial.session!,
          state: "executing",
          operation: { id: "operation-1", tool: "js", startedAt: "2026-10-28T00:01:00.000Z" },
        },
      }),
    );
    setup.server.setLifecycleUnknown(method, true);
    const first = setup.mount();
    fireEvent.click(await screen.findByRole("button", { name: action }));
    await screen.findByRole("button", { name: "Check original outcome" });
    const [originalKey, original] = [...setup.server.originals.entries()][0]!;
    first.unmount();
    first.controller.abort();
    setup.server.setLifecycleUnknown(method, false);
    setup.server.publish({
      ...initial,
      session: {
        ...initial.session!,
        id: "control-2",
        workerId: "worker-2",
        state: "executing",
        operation: { id: "operation-2", tool: "js", startedAt: "2026-10-28T00:02:00.000Z" },
      },
    });
    setup.mount();
    const revoke = await screen.findByRole("button", { name: action });
    await waitFor(() =>
      expect(
        setup.server.calls.some(
          (call) =>
            call.params.method === METHODS.result &&
            (JSON.parse(call.params.input) as { id: string }).id === originalKey,
        ),
      ).toBe(true),
    );
    expect(revoke.hasAttribute("disabled")).toBe(false);
    fireEvent.click(revoke);
    await waitFor(() => expect(setup.server.originals.size).toBe(2));
    const [newKey, current] = [...setup.server.originals.entries()][1]!;
    expect(newKey).not.toBe(originalKey);
    expect(JSON.parse(current.input)).toEqual(
      method === METHODS.stop
        ? { sessionId: "control-2", operationId: "operation-2" }
        : { sessionId: "control-2" },
    );
    expect(setup.server.originals.get(originalKey)).toEqual(original);
    await waitFor(() =>
      expect(
        setup.client.plugins.pendingCommands().map((command) => command.idempotencyKey),
      ).toEqual([originalKey]),
    );
    fireEvent.click(screen.getByRole("button", { name: "Check original outcome" }));
    await screen.findByRole("button", { name: "Check original outcome" });
    expect(setup.server.originals.size).toBe(2);
  },
);

it("refuses a review captured from an older worker rather than silently retargeting it", async () => {
  const setup = rig();
  setup.mount();
  const execute = await review();
  act(() =>
    setup.server.publish({
      ...initial,
      session: { ...initial.session!, workerId: "worker-2", surfaceDigest: "b".repeat(64) },
    }),
  );
  await screen.findByText(/captured target changed/);
  expect(execute.hasAttribute("disabled")).toBe(true);
  expect(setup.server.originals.size).toBe(0);
  fireEvent.click(screen.getByRole("button", { name: "Back to editor" }));
  fireEvent.click(screen.getByRole("button", { name: "Review operation" }));
  fireEvent.click(screen.getByRole("button", { name: "Execute reviewed operation" }));
  await waitFor(() => expect(setup.server.originals.size).toBe(1));
  expect(
    (JSON.parse([...setup.server.originals.values()][0]!.input) as { workerId: string }).workerId,
  ).toBe("worker-2");
});

it("does not allow a retired context read to overwrite the new worker", async () => {
  const setup = rig();
  const held = setup.server.holdContext();
  setup.mount();
  await held.arrived;
  act(() =>
    setup.server.publish({
      ...initial,
      session: { ...initial.session!, workerId: "worker-2", surfaceDigest: "b".repeat(64) },
    }),
  );
  await screen.findByRole("textbox", { name: "Tool arguments (JSON)" });
  await act(async () => held.release());
  fireEvent.click(await review());
  await waitFor(() => expect(setup.server.originals.size).toBe(1));
  expect(
    (JSON.parse([...setup.server.originals.values()][0]!.input) as { workerId: string }).workerId,
  ).toBe("worker-2");
});

it.each(["Refresh status", "Reconnect observation"])(
  "retries a transient context failure for the same worker through %s",
  async (action) => {
    const setup = rig();
    setup.server.setContextUnavailable(true);
    setup.mount();
    expect((await screen.findByRole("alert")).textContent).not.toBe("");
    expect(screen.queryByRole("textbox", { name: "Tool arguments (JSON)" })).toBeNull();
    setup.server.setContextUnavailable(false);
    if (action === "Reconnect observation") {
      act(() => setup.server.endWatch());
      await screen.findByText(/Live observation stopped/);
    }
    fireEvent.click(screen.getByRole("button", { name: action }));
    const editor = await screen.findByRole("textbox", { name: "Tool arguments (JSON)" });
    await waitFor(() => expect(editor.hasAttribute("disabled")).toBe(false));
    expect(setup.server.calls.filter((call) => call.params.method === METHODS.context).length).toBe(
      2,
    );
    expect(setup.server.originals.size).toBe(0);
  },
);

it("does not let an older same-worker context response overwrite an explicit retry", async () => {
  const setup = rig();
  const held = setup.server.holdContext();
  setup.mount();
  await held.arrived;
  setup.server.setInstructions("The current original instructions.");
  fireEvent.click(screen.getByRole("button", { name: "Refresh status" }));
  await screen.findByRole("textbox", { name: "Tool arguments (JSON)" });
  fireEvent.click(screen.getByRole("button", { name: "Original runtime instructions" }));
  await screen.findByText("The current original instructions.");
  await act(async () => held.release());
  expect(screen.queryByText("Use the isolated fixture controls only.")).toBeNull();
  expect(screen.getByText("The current original instructions.")).not.toBeNull();
  expect(setup.server.originals.size).toBe(0);
});

it.each(["metaKey", "ctrlKey"])(
  "selects only the complete captured source with %s and preserves its reading scope",
  async (modifier) => {
    const setup = rig();
    const source = `  Captured instructions\n<script>this is literal source</script>\n${"long source ".repeat(300)}\nTrailing spaces  \n`;
    setup.server.setInstructions(source);
    setup.mount();
    await screen.findByRole("textbox", { name: "Tool arguments (JSON)" });
    fireEvent.click(screen.getByRole("button", { name: "Original runtime instructions" }));
    const reader = screen.getByLabelText("Original runtime instructions");
    expect(reader.textContent).toBe(source);
    window.getSelection()?.removeAllRanges();
    reader.focus();
    expect(fireEvent.keyDown(reader, { key: "a", [modifier]: true })).toBe(false);
    expect(window.getSelection()?.toString()).toBe(source);
    expect(document.activeElement).toBe(reader);
    expect(document.querySelector("script")).toBeNull();
    expect(setup.server.originals.size).toBe(0);
  },
);

it("copies exact captured source and keeps its reader stable while changing wrapping", async () => {
  const setup = rig();
  const source = "  original source\n  spaces and <literal> markup\n";
  setup.server.setInstructions(source);
  const writeText = vi.fn().mockResolvedValue(undefined);
  vi.stubGlobal("navigator", { ...navigator, clipboard: { writeText } });
  onTestFinished(() => {
    vi.unstubAllGlobals();
  });
  setup.mount();
  await screen.findByRole("textbox", { name: "Tool arguments (JSON)" });
  fireEvent.click(screen.getByRole("button", { name: "Original runtime instructions" }));
  const reader = screen.getByLabelText("Original runtime instructions");
  expect(
    within(reader.closest<HTMLElement>('[data-slot="code-block"]')!).getByText(
      "Original runtime instructions",
    ),
  ).not.toBeNull();
  const wrap = screen.getByRole("button", { name: "Enable word wrap" });
  fireEvent.click(wrap);
  expect(screen.getByLabelText("Original runtime instructions")).toBe(reader);
  expect(reader.getAttribute("data-wrap")).toBe("true");
  expect(reader.textContent).toBe(source);
  fireEvent.click(screen.getByRole("button", { name: "Disable word wrap" }));
  expect(reader.getAttribute("data-wrap")).toBe("false");
  fireEvent.click(screen.getByRole("button", { name: "Copy source" }));
  await screen.findByRole("button", { name: "Source copied" });
  expect(writeText).toHaveBeenCalledExactlyOnceWith(source);
  expect(setup.server.originals.size).toBe(0);
});

it("shows captured media and unknown evidence without executing historical code", async () => {
  const setup = rig();
  setup.server.records.set("original-tool", {
    id: "original-tool",
    kind: "tool",
    method: "computer_js",
    admittedAt: "2026-10-28T00:01:00.000Z",
    dispatched: true,
    settled: true,
    inputJSON: '{"code":"historical code"}',
    invocationJSON: '{"origin":"model"}',
    outcomeJSON: JSON.stringify({
      content: [
        { type: "text", text: "Recorded screenshot." },
        { type: "image", mimeType: "image/png", data: png },
        {
          type: "resource",
          resource: {
            uri: "file:///original.html",
            mimeType: "text/html",
            text: "<script>window.historicalExecuted = true</script>",
          },
        },
      ],
    }),
  });
  setup.server.records.set("unknown-tool", {
    id: "unknown-tool",
    kind: "tool",
    method: "computer_js",
    admittedAt: "2026-10-28T00:02:00.000Z",
    dispatched: true,
    settled: false,
    inputJSON: '{"code":"unknown historical code"}',
    invocationJSON: '{"origin":"model"}',
  });
  setup.mount();
  fireEvent.click(await screen.findByRole("button", { name: "original-tool" }));
  const image = await screen.findByRole("img", { name: "Captured image 2" });
  expect(image.getAttribute("src")).toBe(`data:image/png;base64,${png}`);
  expect(screen.getByText("<script>window.historicalExecuted = true</script>")).not.toBeNull();
  expect(document.querySelector("script")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "unknown-tool" }));
  await screen.findByText(/No outcome was confirmed for this original operation/);
  expect(setup.server.originals.size).toBe(0);
});

it("marks a failed observation stale and leaves exact-session revocation available", async () => {
  const setup = rig();
  setup.mount();
  await screen.findByRole("textbox", { name: "Tool arguments (JSON)" });
  act(() => setup.server.endWatch());
  await screen.findByText(/Live observation stopped/);
  expect(screen.getByRole("button", { name: "Review operation" }).hasAttribute("disabled")).toBe(
    true,
  );
  expect(screen.getByRole("button", { name: "End control lease" }).hasAttribute("disabled")).toBe(
    false,
  );
  fireEvent.click(screen.getByRole("button", { name: "Reconnect observation" }));
  await waitFor(() => expect(screen.queryByText(/Live observation stopped/)).toBeNull());
  expect(screen.getByRole("button", { name: "Review operation" }).hasAttribute("disabled")).toBe(
    false,
  );
});

it("rereads the receipt owner after a same-state completion notification", async () => {
  const setup = rig();
  setup.mount();
  await screen.findByText("No commands have been recorded.");
  setup.server.records.set("settled-after-ready", {
    id: "settled-after-ready",
    kind: "tool",
    method: "computer_js",
    admittedAt: "2026-10-28T00:02:00.000Z",
    dispatched: true,
    settled: true,
    inputJSON: '{"code":"original operation"}',
    invocationJSON: '{"origin":"model"}',
    outcomeJSON: '{"content":[{"type":"text","text":"Original result settled."}]}',
  });
  act(() => setup.server.publish(setup.server.snapshot()));
  expect(await screen.findByRole("button", { name: "settled-after-ready" })).not.toBeNull();
  expect(setup.server.originals.size).toBe(0);
});

function recordedTool(id: string): OperationRecord {
  return {
    id,
    kind: "tool",
    method: "computer_js",
    admittedAt: "2026-10-28T00:02:00.000Z",
    dispatched: true,
    settled: true,
    inputJSON: JSON.stringify({ code: "Never execute this historical source." }),
    invocationJSON: '{"origin":"model"}',
    outcomeJSON: JSON.stringify({ content: [{ type: "text", text: `Recorded ${id}.` }] }),
  };
}

it.each(["image", "resource"] as const)(
  "explains an unreadable captured %s while preserving the original outcome",
  async (kind) => {
    const setup = rig();
    const data = "aW52YWxpZCBQTkc=";
    const content =
      kind === "image"
        ? { type: "image", mimeType: "image/png", data }
        : {
            type: "resource",
            resource: { uri: "memory://capture.png", mimeType: "image/png", blob: data },
          };
    const original = {
      ...recordedTool("unreadable-capture"),
      outcomeJSON: JSON.stringify({ content: [content] }),
    };
    setup.server.records.set(original.id, original);
    setup.mount();
    fireEvent.click(await screen.findByRole("button", { name: original.id }));
    const image = await screen.findByRole("img", { name: "Captured image 1" });
    fireEvent.error(image);
    expect(
      await screen.findByRole("img", {
        name: "Captured image 1",
        description: "This recorded image could not be displayed.",
      }),
    ).not.toBeNull();
    expect(image.isConnected).toBe(false);
    const detail = screen.getByRole("region", { name: "Inspected original operation" });
    expect(within(detail).getByText("Recorded outcome")).not.toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Exact recorded outcome" }));
    expect(screen.getByRole("region", { name: "Exact recorded outcome" }).textContent).toBe(
      original.outcomeJSON,
    );
    expect(setup.server.originals.size).toBe(0);
  },
);

it("keeps an adjacent captured image available after another image fails in Chinese", async () => {
  const setup = rig();
  setup.server.records.set("paired-captures", {
    ...recordedTool("paired-captures"),
    outcomeJSON: JSON.stringify({
      content: [
        { type: "image", mimeType: "image/png", data: "aW52YWxpZCBQTkc=" },
        { type: "image", mimeType: "image/png", data: png },
      ],
    }),
  });
  setup.mount("zh");
  fireEvent.click(await screen.findByRole("button", { name: "paired-captures" }));
  const failed = await screen.findByRole("img", { name: "捕获图像 1" });
  const available = screen.getByRole("img", { name: "捕获图像 2" });
  fireEvent.error(failed);
  await screen.findByRole("img", { name: "捕获图像 1", description: "无法显示此记录的图像。" });
  expect(screen.getByRole("img", { name: "捕获图像 2" })).toBe(available);
  expect(available.getAttribute("src")).toBe(`data:image/png;base64,${png}`);
  expect(setup.server.originals.size).toBe(0);
});

it.each(["", " \t\n "])(
  "keeps a resource link named when its optional title is blank (%j)",
  async (title) => {
    const setup = rig();
    const uri = "https://example.invalid/captured-report.md";
    const original = {
      ...recordedTool("named-resource-link"),
      outcomeJSON: JSON.stringify({
        content: [{ type: "resource_link", title, name: "Workspace report", uri }],
      }),
    };
    setup.server.records.set(original.id, original);
    setup.mount();
    const editor = await screen.findByRole("textbox", { name: "Tool arguments (JSON)" });
    const draft = '{"code":"Continue reviewing the original report."}';
    fireEvent.change(editor, { target: { value: draft } });
    fireEvent.click(await screen.findByRole("button", { name: original.id }));
    const link = await screen.findByRole("link", { name: "Workspace report" });
    expect(link.getAttribute("href")).toBe(uri);
    expect(link.getAttribute("target")).toBe("_blank");
    expect(link.getAttribute("rel")).toBe("noopener noreferrer");
    expect(screen.getByText(uri)).not.toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Exact recorded outcome" }));
    expect(screen.getByRole("region", { name: "Exact recorded outcome" }).textContent).toBe(
      original.outcomeJSON,
    );
    expect((editor as HTMLTextAreaElement).value).toBe(draft);
    expect(setup.server.originals.size).toBe(0);
  },
);

it("uses the original URI once when a resource link has no display name", async () => {
  const setup = rig();
  const uri = "https://example.invalid/review/original-resource-without-display-name.md";
  setup.server.records.set("unnamed-resource-link", {
    ...recordedTool("unnamed-resource-link"),
    outcomeJSON: JSON.stringify({
      content: [{ type: "resource_link", title: " \t ", name: " \n ", uri }],
    }),
  });
  setup.mount();
  fireEvent.click(await screen.findByRole("button", { name: "unnamed-resource-link" }));
  expect((await screen.findByRole("link", { name: uri })).getAttribute("href")).toBe(uri);
  expect(screen.getAllByText(uri)).toHaveLength(1);
  expect(setup.server.originals.size).toBe(0);
});

it("does not repeat a resource URI that already names its link", async () => {
  const setup = rig();
  const uri = "https://example.invalid/captured-report.md";
  setup.server.records.set("uri-resource-link", {
    ...recordedTool("uri-resource-link"),
    outcomeJSON: JSON.stringify({ content: [{ type: "resource_link", name: uri, uri }] }),
  });
  setup.mount();
  fireEvent.click(await screen.findByRole("button", { name: "uri-resource-link" }));
  await screen.findByRole("link", { name: uri });
  expect(screen.getAllByText(uri)).toHaveLength(1);
});

it("preserves the original nonblank resource title and destination", async () => {
  const setup = rig();
  const title = "  Workspace report — 原始说明  ";
  const uri = "https://example.invalid/captured-report.md";
  setup.server.records.set("titled-resource-link", {
    ...recordedTool("titled-resource-link"),
    outcomeJSON: JSON.stringify({
      content: [{ type: "resource_link", title, name: "report.md", uri }],
    }),
  });
  setup.mount();
  fireEvent.click(await screen.findByRole("button", { name: "titled-resource-link" }));
  const link = await screen.findByRole("link", { name: "Workspace report — 原始说明" });
  expect(link.textContent).toBe(title);
  expect(link.getAttribute("href")).toBe(uri);
  expect(screen.getAllByText(uri)).toHaveLength(1);
});

it("keeps a named local resource readable without creating a navigation action", async () => {
  const setup = rig();
  const uri = "file:///original/workspace/report.md";
  setup.server.records.set("local-resource-link", {
    ...recordedTool("local-resource-link"),
    outcomeJSON: JSON.stringify({
      content: [{ type: "resource_link", title: "", name: "Local workspace report", uri }],
    }),
  });
  setup.mount();
  fireEvent.click(await screen.findByRole("button", { name: "local-resource-link" }));
  const detail = await screen.findByRole("region", { name: "Inspected original operation" });
  expect(within(detail).getByText("Local workspace report")).not.toBeNull();
  expect(within(detail).getByText(uri)).not.toBeNull();
  expect(within(detail).queryByRole("link")).toBeNull();
  expect(setup.server.originals.size).toBe(0);
});

it.each(["image", "audio"] as const)(
  "keeps a captured %s resource URI and original bytes together",
  async (kind) => {
    const setup = rig();
    const uri = `memory://captures/2026-10-29/retained-original-${kind}`;
    const mimeType = kind === "image" ? "image/png" : "audio/wav";
    const blob = kind === "image" ? png : "UklGRg==";
    const original = {
      ...recordedTool("captured-resource"),
      outcomeJSON: JSON.stringify({
        content: [{ type: "resource", resource: { uri, mimeType, blob } }],
      }),
    };
    setup.server.records.set(original.id, original);
    setup.mount();
    fireEvent.click(await screen.findByRole("button", { name: original.id }));
    const media =
      kind === "image"
        ? await screen.findByRole("img", { name: "Captured image 1" })
        : await screen.findByLabelText("Captured audio 1");
    expect(screen.getByText(uri)).not.toBeNull();
    expect(media.getAttribute("src")).toBe(`data:${mimeType};base64,${blob}`);
    expect(screen.queryByRole("link", { name: uri })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Exact recorded outcome" }));
    expect(screen.getByRole("region", { name: "Exact recorded outcome" }).textContent).toBe(
      original.outcomeJSON,
    );
    expect(setup.server.originals.size).toBe(0);
  },
);

it("keeps a text resource's URI in its existing named source caption", async () => {
  const setup = rig();
  const uri = "memory://captures/notes.txt";
  const text = "Literal captured text.\n";
  setup.server.records.set("text-resource", {
    ...recordedTool("text-resource"),
    outcomeJSON: JSON.stringify({
      content: [{ type: "resource", resource: { uri, mimeType: "text/plain", text } }],
    }),
  });
  setup.mount();
  fireEvent.click(await screen.findByRole("button", { name: "text-resource" }));
  expect((await screen.findByRole("region", { name: uri })).textContent).toBe(text);
  expect(screen.getAllByText(uri)).toHaveLength(1);
  expect(setup.server.originals.size).toBe(0);
});

it.each([undefined, "", " \t "])(
  "names a text resource reader when its URI is absent or blank (%j)",
  async (uri) => {
    const setup = rig();
    const text = " \nOriginal content.\n";
    setup.server.records.set("unnamed-resource", {
      ...recordedTool("unnamed-resource"),
      outcomeJSON: JSON.stringify({
        content: [{ type: "resource", resource: { uri, text } }],
      }),
    });
    setup.mount();
    fireEvent.click(await screen.findByRole("button", { name: "unnamed-resource" }));
    expect((await screen.findByRole("region", { name: "Recorded resource" })).textContent).toBe(
      text,
    );
    expect(setup.server.originals.size).toBe(0);
  },
);

it("reads original evidence next to its expanded record and keeps the editor draft", async () => {
  const setup = rig();
  setup.server.records.set("first-operation", recordedTool("first-operation"));
  setup.server.records.set("second-operation", recordedTool("second-operation"));
  setup.mount();
  const editor = await screen.findByRole("textbox", { name: "Tool arguments (JSON)" });
  const draft = '{"code":"still editing"}';
  fireEvent.change(editor, { target: { value: draft } });
  const first = await screen.findByRole("button", { name: "first-operation" });
  first.focus();
  fireEvent.click(first);
  const evidence = await screen.findByText("Recorded first-operation.");
  expect(first.getAttribute("aria-expanded")).toBe("true");
  expect(document.getElementById(first.getAttribute("aria-controls")!)?.contains(evidence)).toBe(
    true,
  );
  expect(document.activeElement).toBe(first);
  expect((editor as HTMLTextAreaElement).value).toBe(draft);
  fireEvent.click(screen.getByRole("button", { name: "second-operation" }));
  await screen.findByText("Recorded second-operation.");
  expect(first.getAttribute("aria-expanded")).toBe("false");
  expect(screen.queryByText("Recorded first-operation.")).toBeNull();
  expect(setup.server.originals.size).toBe(0);
});

it("retains evidence and refresh focus while rereading the same original record", async () => {
  const setup = rig();
  setup.server.records.set("original-operation", recordedTool("original-operation"));
  setup.mount();
  fireEvent.click(await screen.findByRole("button", { name: "original-operation" }));
  const evidence = await screen.findByText("Recorded original-operation.");
  const refresh = screen.getByRole("button", { name: "Refresh original outcome" });
  const held = setup.server.holdResult();
  refresh.focus();
  fireEvent.click(refresh);
  await held.arrived;
  expect(screen.getByText("Recorded original-operation.")).toBe(evidence);
  expect(document.activeElement).toBe(refresh);
  expect(refresh.getAttribute("aria-disabled")).toBe("true");
  const calls = setup.server.calls.filter((call) => call.params.method === METHODS.result).length;
  fireEvent.click(refresh);
  expect(setup.server.calls.filter((call) => call.params.method === METHODS.result)).toHaveLength(
    calls,
  );
  await act(async () => held.release());
  await waitFor(() => expect(refresh.getAttribute("aria-disabled")).not.toBe("true"));
  expect(document.activeElement).toBe(refresh);
  expect(screen.getByText("Recorded original-operation.")).toBe(evidence);
  expect(setup.server.originals.size).toBe(0);
});

it("keeps later editor focus when an asynchronous original read arrives", async () => {
  const setup = rig();
  setup.server.records.set("delayed-operation", recordedTool("delayed-operation"));
  const held = setup.server.holdResult();
  setup.mount();
  fireEvent.click(await screen.findByRole("button", { name: "delayed-operation" }));
  await held.arrived;
  const editor = screen.getByRole("textbox", { name: "Tool arguments (JSON)" });
  editor.focus();
  fireEvent.change(editor, { target: { value: '{"code":"continued elsewhere"}' } });
  await act(async () => held.release());
  await screen.findByText("Recorded delayed-operation.");
  expect(document.activeElement).toBe(editor);
  expect((editor as HTMLTextAreaElement).value).toBe('{"code":"continued elsewhere"}');
});

it("retires a collapsed read and never reopens it when the old response arrives", async () => {
  const setup = rig();
  setup.server.records.set("retired-operation", recordedTool("retired-operation"));
  const held = setup.server.holdResult();
  setup.mount();
  const trigger = await screen.findByRole("button", { name: "retired-operation" });
  trigger.focus();
  fireEvent.click(trigger);
  await held.arrived;
  fireEvent.click(trigger);
  expect(trigger.getAttribute("aria-expanded")).toBe("false");
  await act(async () => held.release());
  expect(screen.queryByText("Recorded retired-operation.")).toBeNull();
  expect(document.activeElement).toBe(trigger);
  expect(setup.server.originals.size).toBe(0);
});

it("keeps a newer selection when an older original read completes later", async () => {
  const setup = rig();
  setup.server.records.set("older-operation", recordedTool("older-operation"));
  setup.server.records.set("newer-operation", recordedTool("newer-operation"));
  const held = setup.server.holdResult();
  setup.mount();
  const older = await screen.findByRole("button", { name: "older-operation" });
  fireEvent.click(older);
  await held.arrived;
  const newer = screen.getByRole("button", { name: "newer-operation" });
  newer.focus();
  fireEvent.click(newer);
  await screen.findByText("Recorded newer-operation.");
  await act(async () => held.release());
  expect(older.getAttribute("aria-expanded")).toBe("false");
  expect(newer.getAttribute("aria-expanded")).toBe("true");
  expect(screen.queryByText("Recorded older-operation.")).toBeNull();
  expect(document.activeElement).toBe(newer);
});

it("marks retained evidence stale after a failed reread and retries the same original identity", async () => {
  const setup = rig();
  setup.server.records.set("retained-operation", recordedTool("retained-operation"));
  setup.mount();
  fireEvent.click(await screen.findByRole("button", { name: "retained-operation" }));
  const evidence = await screen.findByText("Recorded retained-operation.");
  const refresh = screen.getByRole("button", { name: "Refresh original outcome" });
  setup.server.setResultUnavailable(true);
  refresh.focus();
  fireEvent.click(refresh);
  await screen.findByText(/Retained evidence may be outdated/);
  expect(screen.getByText("Recorded retained-operation.")).toBe(evidence);
  expect(document.activeElement).toBe(refresh);
  setup.server.setResultUnavailable(false);
  fireEvent.click(refresh);
  await waitFor(() => expect(screen.queryByText(/Retained evidence may be outdated/)).toBeNull());
  const reads = setup.server.calls.filter((call) => call.params.method === METHODS.result);
  expect(reads).toHaveLength(3);
  expect(
    reads.every(
      (call) => (JSON.parse(call.params.input) as { id: string }).id === "retained-operation",
    ),
  ).toBe(true);
  expect(setup.server.originals.size).toBe(0);
});

it("retries an initially unreadable original record without inventing an outcome", async () => {
  const setup = rig();
  setup.server.records.set("unreadable-operation", recordedTool("unreadable-operation"));
  setup.server.setResultUnavailable(true);
  setup.mount();
  const trigger = await screen.findByRole("button", { name: "unreadable-operation" });
  trigger.focus();
  fireEvent.click(trigger);
  const detail = await screen.findByRole("region", { name: "Inspected original operation" });
  await within(detail).findByRole("alert");
  expect(screen.queryByText("Recorded unreadable-operation.")).toBeNull();
  expect(screen.queryByText(/Retained evidence may be outdated/)).toBeNull();
  expect(document.activeElement).toBe(trigger);
  setup.server.setResultUnavailable(false);
  fireEvent.click(screen.getByRole("button", { name: "Refresh original outcome" }));
  await within(detail).findByText("Recorded unreadable-operation.");
  expect(setup.server.originals.size).toBe(0);
});

it("returns to the retained original selection after paging without replacing its draft", async () => {
  const setup = rig();
  setup.server.setHistoryPageSize(1);
  setup.server.records.set("page-one", recordedTool("page-one"));
  setup.server.records.set("page-two", recordedTool("page-two"));
  setup.mount();
  const editor = await screen.findByRole("textbox", { name: "Tool arguments (JSON)" });
  fireEvent.change(editor, { target: { value: '{"code":"unsubmitted"}' } });
  fireEvent.click(await screen.findByRole("button", { name: "page-one" }));
  await screen.findByText("Recorded page-one.");
  fireEvent.click(screen.getByRole("button", { name: "Next page" }));
  await screen.findByRole("button", { name: "page-two" });
  expect(screen.queryByText("Recorded page-one.")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Previous page" }));
  const original = await screen.findByRole("button", { name: "page-one" });
  expect(original.getAttribute("aria-expanded")).toBe("true");
  expect(screen.getByText("Recorded page-one.")).not.toBeNull();
  expect((editor as HTMLTextAreaElement).value).toBe('{"code":"unsubmitted"}');
  expect(setup.server.originals.size).toBe(0);
});

it("renders a multi-megabyte captured image without recursive regular-expression matching", async () => {
  const setup = rig();
  const data = Buffer.concat([Buffer.from(png, "base64"), Buffer.alloc(4 * 1024 * 1024)]).toString(
    "base64",
  );
  setup.server.records.set("large-capture", {
    id: "large-capture",
    kind: "tool",
    method: "computer_js",
    admittedAt: "2026-10-28T00:02:00.000Z",
    dispatched: true,
    settled: true,
    inputJSON: '{"code":"original capture"}',
    invocationJSON: '{"origin":"model"}',
    outcomeJSON: JSON.stringify({
      content: [{ type: "image", mimeType: "image/png", data }],
    }),
  });
  setup.mount();
  fireEvent.click(await screen.findByRole("button", { name: "large-capture" }));
  const image = await screen.findByRole("img", { name: "Captured image 1" });
  expect(image.getAttribute("src")?.length).toBe("data:image/png;base64,".length + data.length);
  expect(setup.server.originals.size).toBe(0);
});

it("renders the operator boundary and controls in Chinese", async () => {
  const setup = rig(fixture({ ...initial, session: undefined }));
  setup.mount("zh");
  const panel = await screen.findByRole("region", { name: "电脑控制" });
  expect(within(panel).getByText(/操作员控制台必须位于被控桌面/)).not.toBeNull();
  expect(
    within(panel).getByRole("button", { name: "授权并打开控制" }).hasAttribute("disabled"),
  ).toBe(false);
});
