import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { type CallPluginRequest, type Model, type Page } from "@flame/runtime-client/wire";
import { stringifyJSON } from "@qimu/sdk/json";
import { createFlameClient } from "@flame/runtime-client";
import { createMemoryTransport } from "@flame/runtime-client/testing";
import { respondSuccess, waitForRequest } from "@flame/runtime-client/testing";
import { type ComponentContext } from "@flame/component-sdk";
import { Schedules } from "../../../../plugins/schedules/component/component";
import {
  CAPABILITY_JSON,
  type MethodInputs,
} from "../../../../plugins/schedules/package/io.github.tangerg.flame/contracts/methods.generated";
import { VISUAL_CAPABILITIES } from "../../visual/agentFixtureFacts";
import { VISUAL_SCHEDULES_PAGE } from "../../visual/componentSnapshots";
let transport: ReturnType<typeof createMemoryTransport>;

let client: ReturnType<typeof createFlameClient>;

let owner: AbortController;

let context: ComponentContext;

beforeEach(() => {
  transport = createMemoryTransport();
  client = createFlameClient(transport, { capabilities: () => VISUAL_CAPABILITIES });
  owner = new AbortController();
  context = {
    component: {
      installationId: "e4407303-68e3-495d-86d6-9c3f575a51ed",
      digest: "a".repeat(64),
      componentId: "test",
    },
    compose() {
      throw new Error("Composition is not used by this fixture");
    },
    runtime: client,
    locale: "en",
    signal: owner.signal,
    binding: {
      installationId: "e4407303-68e3-495d-86d6-9c3f575a51ed",
      digest: "a".repeat(64),
      contribution: "schedules-provider",
      contract: CAPABILITY_JSON,
    },
    telemetry: { read: () => ({ spans: [], metrics: [], logs: [] }), clear() {} },
    track: (operation) => operation(context.signal),
  };
});

afterEach(async () => {
  cleanup();
  owner.abort();
  await client.close();
});

async function loadSchedules(rows: unknown[] = []) {
  testClient.render(<Schedules context={context} />);
  const read = await waitForRequest(transport, "plugins.call");
  await act(async () =>
    respondSuccess(transport, read.id, { output: stringifyJSON({ Rows: rows, NextCursor: "" }) }),
  );
}

async function openEditor(rows: unknown[] = []) {
  await loadSchedules(rows);
  if (rows.length) {
    fireEvent.click(screen.getByRole("button", { name: /^More actions for / }));
    fireEvent.click(await screen.findByRole("menuitem", { name: "Edit" }));
  } else {
    fireEvent.click(screen.getByRole("button", { name: "Add schedule" }));
  }
  fireEvent.change(screen.getByRole("textbox", { name: "Title" }), {
    target: { value: "Retained draft" },
  });
  return waitForRequest(transport, "providers.list");
}

async function scheduleRequest(method: keyof MethodInputs, after = 0) {
  const matching = () =>
    transport
      .outbox()
      .slice(after)
      .filter(
        (request) =>
          request.method === "plugins.call" &&
          (request.params as CallPluginRequest).method === method,
      );
  await waitFor(() => expect(matching().length).toBeGreaterThan(0));
  return matching()[0]!;
}

async function providerModels() {
  await waitFor(() =>
    expect(transport.outbox().filter((request) => request.method === "models.list")).toHaveLength(
      2,
    ),
  );
  return transport.outbox().filter((request) => request.method === "models.list");
}

function refuse(id: string) {
  transport.inject({
    jsonrpc: "2.0",
    id,
    error: {
      code: -32603,
      message: "internal_error",
      data: { type: "internal_error", detail: "catalog unavailable" },
    },
  });
}

function choose(label: string) {
  const choice = screen.getByRole("option", { name: label });
  fireEvent.pointerDown(choice, { pointerType: "mouse" });
  fireEvent.click(choice);
}

it("preserves independent catalogs and known choices during failed refreshes", async () => {
  const providers = await openEditor();
  const configured = {
    data: ["healthy", "failed"].map((id) => ({
      id,
      configured: true,
      credentialRequirement: "apiKeyRequired",
    })),
  };
  await act(async () => respondSuccess(transport, providers.id, configured));
  const requests = await providerModels();
  await act(async () => {
    respondSuccess(transport, requests[0]!.id, { data: [{ id: "known", provider: "healthy" }] });
    refuse(requests[1]!.id);
  });
  expect((await screen.findByRole("alert")).textContent).toContain("failed: catalog unavailable");
  fireEvent.click(screen.getByRole("combobox", { name: "Model" }));
  expect(await screen.findByRole("option", { name: "healthy / known" })).toBeTruthy();
  expect(screen.getByRole("option", { name: "Runtime default model" })).toBeTruthy();
  choose("Runtime default model");
  fireEvent.click(screen.getByRole("button", { name: "Refresh model choices" }));
  await waitFor(() =>
    expect(
      transport.outbox().filter((request) => request.method === "providers.list"),
    ).toHaveLength(2),
  );
  const again = transport
    .outbox()
    .filter((request) => request.method === "providers.list")
    .at(-1)!;
  await act(async () => respondSuccess(transport, again.id, configured));
  await waitFor(() =>
    expect(transport.outbox().filter((request) => request.method === "models.list")).toHaveLength(
      4,
    ),
  );
  const refreshed = transport
    .outbox()
    .filter((request) => request.method === "models.list")
    .slice(-2);
  await act(async () => {
    refuse(refreshed[0]!.id);
    respondSuccess(transport, refreshed[1]!.id, {
      data: [{ id: "recovered", provider: "failed" }],
    });
  });
  await waitFor(() =>
    expect(screen.getByRole("alert").textContent).toContain("healthy: catalog unavailable"),
  );
  fireEvent.click(screen.getByRole("combobox", { name: "Model" }));
  expect(await screen.findByRole("option", { name: "healthy / known" })).toBeTruthy();
  expect(screen.getByRole("option", { name: "failed / recovered" })).toBeTruthy();
  expect((screen.getByRole("textbox", { name: "Title" }) as HTMLInputElement).value).toBe(
    "Retained draft",
  );
});

it.each(["active", "missing", "deprecated"])(
  "represents an existing %s selection without changing its identity",
  async (state) => {
    const item = {
      ID: "schedule",
      Revision: 1,
      Title: "Original",
      Instructions: "Read",
      Cron: "0 9 * * *",
      CWD: "",
      NextRunAt: "0001-01-01T00:00:00Z",
      LastRunAt: "0001-01-01T00:00:00Z",
      CreatedAt: "2026-10-16T00:00:00Z",
      ModelSelection: { provider: "original", model: "selected", reasoningEffort: "" },
    };
    const providers = await openEditor([item]);
    await act(async () =>
      respondSuccess(transport, providers.id, {
        data: [{ id: "original", configured: true, credentialRequirement: "apiKeyRequired" }],
      }),
    );
    const request = await waitForRequest(transport, "models.list");
    await act(async () =>
      respondSuccess(transport, request.id, {
        data:
          state === "missing"
            ? []
            : [{ id: "selected", provider: "original", deprecated: state === "deprecated" }],
      }),
    );
    const label = `original / selected${state === "active" ? "" : state === "missing" ? " (unavailable)" : " (deprecated)"}`;
    await waitFor(() =>
      expect(screen.getByRole("combobox", { name: "Model" }).textContent).toBe(label),
    );
    fireEvent.click(screen.getByRole("combobox", { name: "Model" }));
    expect(await screen.findByRole("option", { name: label })).toBeTruthy();
    choose("Runtime default model");
    await waitFor(() =>
      expect(screen.getByRole("combobox", { name: "Model" }).textContent).toBe(
        "Runtime default model",
      ),
    );
  },
);

it("ignores a retired context's late catalog after replacement", async () => {
  const old = Promise.withResolvers<Page<Model>>();
  const configured = vi.fn(async () => ({
    data: [{ id: "provider", configured: true, credentialRequirement: "apiKeyRequired" as const }],
  }));
  const modelList = vi.fn(() => old.promise);
  const captured = {
    ...context,
    runtime: {
      ...client,
      providers: { ...client.providers, list: configured },
      models: { ...client.models, list: modelList },
    },
  };
  const view = testClient.render(<Schedules context={captured} />);
  const first = await waitForRequest(transport, "plugins.call");
  await act(async () =>
    respondSuccess(transport, first.id, { output: stringifyJSON({ Rows: [], NextCursor: "" }) }),
  );
  await waitFor(() => expect(modelList).toHaveBeenCalledTimes(1));
  const replacement = {
    ...captured,
    runtime: {
      ...captured.runtime,
      models: {
        ...client.models,
        list: async () => ({ data: [{ id: "current", provider: "provider" }] }),
      },
    },
  };
  view.rerender(<Schedules context={replacement} />);
  fireEvent.click(screen.getByRole("button", { name: "Add schedule" }));
  await waitFor(() =>
    expect(
      (screen.getByRole("button", { name: "Refresh model choices" }) as HTMLButtonElement).disabled,
    ).toBe(false),
  );
  await act(async () => old.resolve({ data: [{ id: "retired", provider: "provider" }] }));
  fireEvent.click(screen.getByRole("combobox", { name: "Model" }));
  expect(await screen.findByRole("option", { name: "provider / current" })).toBeTruthy();
  expect(screen.queryByRole("option", { name: "provider / retired" })).toBeNull();
});

it("reads models for each configured provider and preserves a draft across catalog arrival", async () => {
  const providers = await openEditor();
  await act(async () =>
    respondSuccess(transport, providers.id, {
      data: [
        { id: "openai", configured: true, credentialRequirement: "apiKeyRequired" },
        { id: "anthropic", configured: true, credentialRequirement: "apiKeyRequired" },
        { id: "unconfigured", configured: false, credentialRequirement: "apiKeyRequired" },
      ],
    }),
  );
  await waitFor(() =>
    expect(transport.outbox().filter((request) => request.method === "models.list")).toHaveLength(
      2,
    ),
  );
  const requests = transport.outbox().filter((request) => request.method === "models.list");
  expect(requests.map((request) => request.params)).toEqual([
    { provider: "openai" },
    { provider: "anthropic" },
  ]);
  await act(async () => {
    for (const request of requests) {
      const provider = (request.params as { provider: string }).provider;
      respondSuccess(transport, request.id, {
        data: [{ id: "shared-model", provider, displayName: "Shared model" }],
      });
    }
  });
  expect((screen.getByRole("textbox", { name: "Title" }) as HTMLInputElement).value).toBe(
    "Retained draft",
  );
  fireEvent.click(screen.getByRole("combobox", { name: "Model" }));
  expect(await screen.findByRole("option", { name: "openai / Shared model" })).toBeTruthy();
  expect(screen.getByRole("option", { name: "anthropic / Shared model" })).toBeTruthy();
  const choice = screen.getByRole("option", { name: "anthropic / Shared model" });
  fireEvent.pointerDown(choice, { pointerType: "mouse" });
  fireEvent.click(choice);
  await waitFor(() =>
    expect(screen.getByRole("combobox", { name: "Model" }).textContent).toContain(
      "anthropic / Shared model",
    ),
  );
  expect(screen.queryByRole("alert")).toBeNull();
});

it("shows a catalog refusal without discarding the editor", async () => {
  const providers = await openEditor();
  await act(async () =>
    transport.inject({
      jsonrpc: "2.0",
      id: providers.id,
      error: {
        code: -32603,
        message: "internal_error",
        data: { type: "internal_error", detail: "provider catalog unavailable" },
      },
    }),
  );
  expect((await screen.findByRole("alert")).textContent).toBe(
    "Model choices could not be read: provider catalog unavailable",
  );
  expect((screen.getByRole("textbox", { name: "Title" }) as HTMLInputElement).value).toBe(
    "Retained draft",
  );
});

it("keeps a pending save in its task and preserves the draft after rejection", async () => {
  const schedule = VISUAL_SCHEDULES_PAGE.Rows[0]!;
  const providers = await openEditor([schedule]);
  await act(async () => respondSuccess(transport, providers.id, { data: [] }));
  const editor = screen.getByRole("dialog", { name: `Edit ${schedule.Title}` });
  const title = within(editor).getByRole<HTMLInputElement>("textbox", { name: "Title" });
  fireEvent.click(within(editor).getByRole("button", { name: "Save schedule" }));
  const request = await scheduleRequest("flame.schedules/update.v1");
  expect(JSON.parse((request.params as CallPluginRequest).input)).toMatchObject({
    ID: schedule.ID,
    ExpectedRevision: schedule.Revision,
    Title: "Retained draft",
    ModelSelection: {
      Provider: schedule.ModelSelection.provider,
      Model: schedule.ModelSelection.model,
      ReasoningEffort: schedule.ModelSelection.reasoningEffort,
    },
  });
  fireEvent.keyDown(editor, { key: "Escape" });
  fireEvent.click(within(editor).getByRole("button", { name: "Cancel" }));
  fireEvent.click(within(editor).getByRole("button", { name: "Close schedule editor" }));
  expect(screen.getByRole("dialog")).toBe(editor);
  expect(title.disabled).toBe(true);
  expect(within(editor).getByRole("status").textContent).toBe("Save schedule…");
  await act(async () =>
    respondSuccess(transport, request.id, {
      failure: { code: "invalid", message: "The schedule was refused." },
    }),
  );
  expect((await within(editor).findByRole("status")).textContent).toBe("The schedule was refused.");
  expect(title.value).toBe("Retained draft");
  expect(title.disabled).toBe(false);
  expect(
    within(editor).getByRole<HTMLButtonElement>("button", { name: "Save schedule" }).disabled,
  ).toBe(false);
  fireEvent.click(within(editor).getByRole("button", { name: "Cancel" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  await waitFor(() =>
    expect(document.activeElement).toBe(
      screen.getByRole("searchbox", { name: "Search this page" }),
    ),
  );
});

it("returns an acknowledged save to retained search while its record refreshes", async () => {
  const schedule = VISUAL_SCHEDULES_PAGE.Rows[0]!;
  const providers = await openEditor([schedule]);
  await act(async () => respondSuccess(transport, providers.id, { data: [] }));
  const opener = screen.getByRole("button", {
    name: `More actions for ${schedule.Title}`,
    hidden: true,
  });
  const editor = screen.getByRole("dialog", { name: `Edit ${schedule.Title}` });
  fireEvent.submit(editor);
  const request = await scheduleRequest("flame.schedules/update.v1");
  const after = transport.outbox().length;
  const updated = { ...schedule, Title: "Retained draft", Revision: 2 };
  await act(async () => respondSuccess(transport, request.id, { output: stringifyJSON(updated) }));
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect((opener as HTMLButtonElement).disabled).toBe(true);
  const search = screen.getByRole("searchbox", { name: "Search this page" });
  await waitFor(() => expect(document.activeElement).toBe(search));
  expect(screen.getByText("Save schedule completed.")).toBeTruthy();
  const refresh = await scheduleRequest("flame.schedules/list.v1", after);
  await act(async () =>
    respondSuccess(transport, refresh.id, {
      output: stringifyJSON({ Rows: [updated], NextCursor: "" }),
    }),
  );
  expect(await screen.findByRole("heading", { name: updated.Title })).toBeTruthy();
  expect(document.activeElement).toBe(search);
  fireEvent.click(screen.getByRole("button", { name: "Add schedule" }));
  const next = await screen.findByRole("dialog", { name: "New schedule" });
  expect(within(next).queryByText("Save schedule completed.")).toBeNull();
});

it("returns creation and cancellation to Add while a catalog read is pending", async () => {
  const providers = await openEditor();
  await act(async () => respondSuccess(transport, providers.id, { data: [] }));
  const add = screen.getByRole("button", { name: "Add schedule", hidden: true });
  fireEvent.change(screen.getByRole("textbox", { name: "Instructions" }), {
    target: { value: "Review the workspace." },
  });
  fireEvent.change(screen.getByRole("textbox", { name: "Cron expression" }), {
    target: { value: "0 9 * * 1" },
  });
  fireEvent.submit(screen.getByRole("dialog", { name: "New schedule" }));
  const request = await scheduleRequest("flame.schedules/create.v1");
  const after = transport.outbox().length;
  await act(async () =>
    respondSuccess(transport, request.id, {
      output: stringifyJSON({ ...VISUAL_SCHEDULES_PAGE.Rows[1], Title: "Retained draft" }),
    }),
  );
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  await waitFor(() => expect(document.activeElement).toBe(add));
  await scheduleRequest("flame.schedules/list.v1", after);
  expect(screen.getByText("Reading schedules…")).toBeTruthy();
  fireEvent.click(add);
  const editor = await screen.findByRole("dialog", { name: "New schedule" });
  fireEvent.keyDown(within(editor).getByRole("textbox", { name: "Title" }), { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  await waitFor(() => expect(document.activeElement).toBe(add));
});

it("retains an uncertain save and reconciles only the original command", async () => {
  const send = vi.spyOn(transport, "send");
  const call = vi.spyOn(client.plugins, "call");
  const schedule = VISUAL_SCHEDULES_PAGE.Rows[0]!;
  const providers = await openEditor([schedule]);
  await act(async () => respondSuccess(transport, providers.id, { data: [] }));
  const editor = screen.getByRole("dialog", { name: `Edit ${schedule.Title}` });
  fireEvent.submit(editor);
  const first = await scheduleRequest("flame.schedules/update.v1");
  const original = send.mock.calls.find(([request]) => request.id === first.id)!;
  await act(async () => respondSuccess(transport, first.id, { output: 42 }));
  expect((await within(editor).findByRole("alert")).textContent).toContain(
    "The original outcome is not confirmed.",
  );
  expect(within(editor).getByText(`Original command: ${original[2]!.idempotencyKey}`)).toBeTruthy();
  fireEvent.keyDown(editor, { key: "Escape" });
  fireEvent.click(within(editor).getByRole("button", { name: "Cancel" }));
  expect(screen.getByRole("dialog")).toBe(editor);
  expect(within(editor).getByRole<HTMLInputElement>("textbox", { name: "Title" }).value).toBe(
    "Retained draft",
  );
  const after = transport.outbox().length;
  fireEvent.click(within(editor).getByRole("button", { name: "Reconcile original command" }));
  const retry = await scheduleRequest("flame.schedules/update.v1", after);
  expect(retry.params).toEqual(first.params);
  const repeated = send.mock.calls.find(([request]) => request.id === retry.id)!;
  expect(repeated[2]!.idempotencyKey).toBe(original[2]!.idempotencyKey);
  expect(repeated[2]!.idempotencyNamespace).toBe(original[2]!.idempotencyNamespace);
  expect(
    call.mock.calls.filter(([request]) => request.method === "flame.schedules/update.v1"),
  ).toHaveLength(1);
  await act(async () => respondSuccess(transport, retry.id, { output: stringifyJSON(schedule) }));
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(screen.getByText("Save schedule completed.")).toBeTruthy();
});

it("returns to retained search after an acknowledged deletion removes its record", async () => {
  const schedule = VISUAL_SCHEDULES_PAGE.Rows[0]!;
  await loadSchedules([schedule]);
  const search = screen.getByRole<HTMLInputElement>("searchbox", { name: "Search this page" });
  fireEvent.change(search, { target: { value: "nightly" } });
  fireEvent.click(screen.getByRole("button", { name: `More actions for ${schedule.Title}` }));
  fireEvent.click(await screen.findByRole("menuitem", { name: "Delete" }));
  const dialog = await screen.findByRole("alertdialog", { name: `Delete ${schedule.Title}?` });
  fireEvent.click(within(dialog).getByRole("button", { name: "Confirm deletion" }));
  const request = await scheduleRequest("flame.schedules/delete.v1");
  const after = transport.outbox().length;
  await act(async () => respondSuccess(transport, request.id, { output: "{}" }));
  await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull());
  await waitFor(() => expect(document.activeElement).toBe(search));
  const refresh = await scheduleRequest("flame.schedules/list.v1", after);
  await act(async () =>
    respondSuccess(transport, refresh.id, { output: stringifyJSON({ Rows: [], NextCursor: "" }) }),
  );
  expect(screen.queryByRole("heading", { name: schedule.Title })).toBeNull();
  expect(document.activeElement).toBe(search);
  expect(search.value).toBe("nightly");
});

it("resolves a removed editor opener to search after a canonical refresh", async () => {
  const schedule = VISUAL_SCHEDULES_PAGE.Rows[0]!;
  const providers = await openEditor([schedule]);
  await act(async () => respondSuccess(transport, providers.id, { data: [] }));
  const opener = screen.getByRole("button", {
    name: `More actions for ${schedule.Title}`,
    hidden: true,
  });
  const subscription = await waitForRequest(transport, "runtime.subscribe");
  const after = transport.outbox().length;
  await act(async () => respondSuccess(transport, subscription.id, {}));
  const refresh = await scheduleRequest("flame.schedules/list.v1", after);
  await act(async () =>
    respondSuccess(transport, refresh.id, { output: stringifyJSON({ Rows: [], NextCursor: "" }) }),
  );
  expect(opener.isConnected).toBe(false);
  fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  await waitFor(() =>
    expect(document.activeElement).toBe(
      screen.getByRole("searchbox", { name: "Search this page" }),
    ),
  );
});
