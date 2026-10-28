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
import { methods } from "../../../../plugins/computer/component/useComputer";
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
  let blocked: { resolve(value: Response): void; id: string | number } | undefined;
  let unavailableLookup = false;
  let unavailableContext = false;
  let instructions = "Use the isolated fixture controls only.";
  const unknownLifecycleMethods = new Set<string>();
  const unknownLifecycleCommands = new Set<string>();
  let heldContext: { arrived: Promise<void>; release(): void } | undefined;
  let contextGate: Promise<void> | undefined;
  let contextArrived: (() => void) | undefined;

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
      if (params.method !== methods.watch)
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
    if (params.method === methods.read) return output(snapshot);
    if (params.method === methods.context) {
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
    if (params.method === methods.history)
      return output({
        entries: [...records.values()].map(
          ({ inputJSON: _input, invocationJSON: _invocation, outcomeJSON: _outcome, ...entry }) =>
            entry,
        ),
      });
    if (params.method === methods.result) {
      const record = records.get((JSON.parse(params.input) as { id: string }).id);
      if (!record) throw new Error("fixture operation record is missing");
      return output(record);
    }
    if (!key) throw new Error("fixture command has no original identity");
    const input = JSON.parse(params.input) as Record<string, unknown>;
    if (originals.has(key)) {
      if (JSON.stringify(params) !== JSON.stringify(originals.get(key)))
        throw new Error("fixture command replay changed its captured input");
      if (unknownLifecycleCommands.has(key)) return unknown();
      if (executeMode === "unknown" && params.method === methods.execute) return unknown();
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
    if (params.method === methods.open) {
      publish({ ...snapshot, session: structuredClone(initial.session) });
      result = snapshot.session;
    } else if (params.method === methods.close || params.method === methods.stop) {
      if (input.sessionId !== snapshot.session?.id)
        throw new Error("fixture command changed its captured session");
      if (params.method === methods.stop && input.operationId !== snapshot.session?.operation?.id)
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
    } else if (params.method === methods.reply) {
      if (input.requestId !== snapshot.session?.approval?.id)
        return success({
          failure: { code: "permission_retired", message: "the original permission request ended" },
        });
      publish({ ...snapshot, session: { ...snapshot.session!, approval: undefined } });
      result = true;
    } else if (params.method === methods.execute) {
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
          blocked = { resolve, id: request.id };
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
  fireEvent.change(lease, { target: { value: "30.5" } });
  fireEvent.click(screen.getByRole("button", { name: "Authorize and open control" }));
  expect((await screen.findByRole("alert")).textContent).toContain("60 to 3600");
  expect(setup.server.originals.size).toBe(0);
  fireEvent.change(lease, { target: { value: "300" } });
  fireEvent.click(screen.getByRole("button", { name: "Authorize and open control" }));
  await screen.findByRole("button", { name: "End control lease" });
  const [original] = setup.server.originals.values();
  expect(original!.method).toBe(methods.open);
  expect(JSON.parse(original!.input)).toEqual({ leaseSeconds: 300 });
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
      "disabled",
    ),
  ).toBe(true);
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
      .filter((call) => call.params.method === methods.execute)
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
  expect(cancellation![1].method).toBe(methods.stop);
  expect(JSON.parse(cancellation![1].input)).toEqual({
    sessionId: "control-1",
    operationId: execution![0],
  });
  expect(await screen.findByText("Closed", { exact: true })).not.toBeNull();
});

it.each([
  { method: methods.close, action: "End control lease" },
  { method: methods.stop, action: "Stop current operation" },
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
            call.params.method === methods.result &&
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
      method === methods.stop
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
    expect(setup.server.calls.filter((call) => call.params.method === methods.context).length).toBe(
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
