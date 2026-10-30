import { unavailableComponentNotifications } from "@/test/unavailableNotifications";
import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { type ComponentContext } from "@flame/component-sdk";
import { createFlameClient } from "@flame/runtime-client";
import { createMemoryTransport } from "@flame/runtime-client/testing";
import { PROBLEM_CODES } from "@flame/runtime-client/wire";
import sample from "@flame/runtime-client/testing/samples/session.json";
import { RenameSession } from "../../../plugins/trajectory/component/component";
import { VISUAL_CAPABILITIES } from "./agentFixtureFacts";

const EXACT_TITLE = "会话  标题 draft";
const BUSY = {
  code: PROBLEM_CODES.session_busy,
  message: "session_busy",
  data: { type: "session_busy" },
};

type MemoryTransport = ReturnType<typeof createMemoryTransport>;

const releases: Array<() => Promise<void>> = [];

afterEach(async () => {
  cleanup();
  await Promise.all(releases.splice(0).map((release) => release()));
  vi.restoreAllMocks();
});

function mountRename() {
  const transport = createMemoryTransport();
  const send = vi.spyOn(transport, "send");
  const client = createFlameClient(transport, { capabilities: () => VISUAL_CAPABILITIES });
  const owner = new AbortController();
  const tracked: Promise<unknown>[] = [];
  const context: ComponentContext = {
    component: { installationId: "trajectory", digest: "c".repeat(64), componentId: "rename" },
    runtime: client,
    sessionId: "ses_1",
    sessionTarget: Object.freeze({ id: "ses_1", title: "Original title", revision: 7 }),
    telemetry: { read: () => ({ spans: [], metrics: [], logs: [] }), clear() {} },
    locale: "en",
    signal: owner.signal,
    track: (operation) => {
      owner.signal.throwIfAborted();
      const original = operation(owner.signal);
      tracked.push(original);
      return original;
    },
    saveFile: async () => "requested",
    notifications: unavailableComponentNotifications,
    subscribeRootRunSettlements() {
      throw new Error("This fixture must not observe run settlements");
    },
    registerCommand() {
      throw new Error("This fixture must not register client commands");
    },
    compose: () => null,
  };
  const dispose = async () => {
    owner.abort();
    cleanup();
    await client.close();
  };
  releases.push(dispose);
  testClient.render(
    <>
      <RenameSession context={context} />
      <button type="button">Later focus</button>
    </>,
  );
  const field = screen.getByRole("textbox", { name: "Session title" }) as HTMLInputElement;
  return {
    transport,
    send,
    owner,
    tracked,
    field,
    form: field.closest("form")!,
    save: () => screen.getByRole("button", { name: /Save title|Saving title/ }),
    later: screen.getByRole("button", { name: "Later focus" }),
  };
}

const updates = (transport: MemoryTransport) =>
  transport.outbox().filter((request) => request.method === "sessions.update");

async function update(transport: MemoryTransport, ordinal: number) {
  await waitFor(() => expect(updates(transport)).toHaveLength(ordinal));
  return updates(transport)[ordinal - 1]!;
}

async function settle() {
  await act(async () => {
    await Promise.resolve();
  });
}

it("holds a field-origin save pending, refuses repeats, keeps the draft after refusal and saves on retry", async () => {
  const { transport, field, form, save } = mountRename();
  fireEvent.change(field, { target: { value: EXACT_TITLE } });
  field.focus();
  fireEvent.submit(form);

  const first = await update(transport, 1);
  expect(first.params).toMatchObject({
    sessionId: "ses_1",
    expectedRevision: 7,
    title: EXACT_TITLE,
  });
  expect(field.readOnly).toBe(true);
  expect(field.disabled).toBe(false);
  expect(field.getAttribute("aria-disabled")).toBe("true");
  expect(document.activeElement).toBe(field);
  expect(save().getAttribute("aria-busy")).toBe("true");
  expect(screen.getByRole("status").textContent).toBe("Saving title…");
  save().focus();
  expect(document.activeElement).toBe(save());
  field.focus();

  fireEvent.submit(form);
  fireEvent.click(save());
  fireEvent.keyDown(save(), { key: "Enter" });
  await settle();
  expect(updates(transport)).toHaveLength(1);

  await act(async () => transport.inject({ jsonrpc: "2.0", id: first.id, error: BUSY }));
  await screen.findByRole("alert");
  expect(field.value).toBe(EXACT_TITLE);
  expect(field.readOnly).toBe(false);
  expect(field.getAttribute("aria-disabled")).toBeNull();
  expect(document.activeElement).toBe(field);

  fireEvent.change(field, { target: { value: "Retried title" } });
  fireEvent.submit(form);
  const retried = await update(transport, 2);
  expect(retried.params).toMatchObject({ expectedRevision: 7, title: "Retried title" });
  await act(async () =>
    transport.inject({
      jsonrpc: "2.0",
      id: retried.id,
      result: { ...sample, id: "ses_1", title: "Canonical title", revision: 8 },
    }),
  );
  await screen.findByText(/Saved ses_1 · Revision 8 · Canonical title\./);
  expect(field.readOnly).toBe(true);
  expect(save().getAttribute("aria-disabled")).toBe("true");

  fireEvent.submit(form);
  await settle();
  expect(updates(transport)).toHaveLength(2);
});

it("leaves focus where the user moved it when an action-origin save is refused", async () => {
  const { transport, field, save, later } = mountRename();
  save().focus();
  fireEvent.click(save());

  const request = await update(transport, 1);
  expect(document.activeElement).toBe(save());
  expect(save().getAttribute("aria-busy")).toBe("true");

  later.focus();
  await act(async () => transport.inject({ jsonrpc: "2.0", id: request.id, error: BUSY }));
  await screen.findByRole("alert");
  expect(document.activeElement).toBe(later);
  expect(field.value).toBe("Original title");
});

it("reconciles an unconfirmed save through the original command identity", async () => {
  const { transport, send, field, form, save } = mountRename();
  fireEvent.submit(form);
  const original = await update(transport, 1);
  await act(async () =>
    transport.inject({
      jsonrpc: "2.0",
      id: original.id,
      error: { code: -32000, message: "garbled" },
    }),
  );

  await screen.findByText(/The change is unconfirmed/);
  expect(field.readOnly).toBe(true);
  fireEvent.submit(form);
  await settle();
  expect(updates(transport)).toHaveLength(1);
  expect(save().getAttribute("aria-disabled")).toBe("true");

  const reconcile = screen.getByRole("button", { name: "Reconcile original command" });
  reconcile.focus();
  fireEvent.click(reconcile);
  const replay = await update(transport, 2);
  expect(replay.params).toEqual(original.params);
  const keys = send.mock.calls
    .filter(([request]) => request.method === "sessions.update")
    .map(([, , options]) => options?.idempotencyKey);
  expect(keys).toHaveLength(2);
  expect(keys[0]).toEqual(expect.any(String));
  expect(keys[1]).toBe(keys[0]);

  const checking = screen.getByRole("button", { name: /Checking original command/ });
  expect(checking.getAttribute("aria-busy")).toBe("true");
  expect(screen.getByText("Checking original command…").getAttribute("role")).toBe("status");
  expect(document.activeElement).toBe(checking);
  fireEvent.click(checking);
  fireEvent.submit(form);
  await settle();
  expect(updates(transport)).toHaveLength(2);

  await act(async () =>
    transport.inject({
      jsonrpc: "2.0",
      id: replay.id,
      result: { ...sample, id: "ses_1", title: "Canonical title", revision: 8 },
    }),
  );
  await screen.findByText(/Saved ses_1 · Revision 8 · Canonical title\./);
  expect(screen.queryByText(/The change is unconfirmed/)).toBeNull();
});

it("projects nothing from a response that arrives after the action closes", async () => {
  const errors = vi.spyOn(console, "error");
  const { transport, form, owner, tracked } = mountRename();
  fireEvent.submit(form);
  const request = await update(transport, 1);

  owner.abort();
  cleanup();
  transport.inject({
    jsonrpc: "2.0",
    id: request.id,
    result: { ...sample, id: "ses_1", title: "Late title", revision: 8 },
  });
  await Promise.allSettled(tracked);
  await Promise.resolve();

  expect(screen.queryByText(/Saved/)).toBeNull();
  expect(screen.queryByRole("alert")).toBeNull();
  expect(errors).not.toHaveBeenCalled();
});
