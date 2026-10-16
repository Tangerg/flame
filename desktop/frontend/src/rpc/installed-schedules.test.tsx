import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { Model, Page } from "@flame/runtime-contract/wire";
import { createFlameClient, stringifyJSON } from "@flame/runtime-contract/client";
import { createMemoryTransport } from "@flame/runtime-contract/client/transports/memory";
import {
  respondSuccess,
  waitForRequest,
} from "@flame/runtime-contract/client/transports/memory.testkit";
import type { ComponentContext } from "@flame/plugin-sdk";
import { Schedules } from "../../../../plugins/schedules/src/component";
import { CAPABILITY_JSON } from "../../../../plugins/schedules/io.github.tangerg.flame/contracts/methods.generated";

let transport: ReturnType<typeof createMemoryTransport>;
let client: ReturnType<typeof createFlameClient>;
let owner: AbortController;
let context: ComponentContext;

beforeEach(() => {
  transport = createMemoryTransport();
  client = createFlameClient(transport);
  owner = new AbortController();
  context = {
    runtime: client,
    signal: owner.signal,
    binding: {
      installationId: "e4407303-68e3-495d-86d6-9c3f575a51ed",
      digest: "a".repeat(64),
      contribution: "schedules-provider",
      contract: CAPABILITY_JSON,
    },
    telemetry: { read: () => ({ spans: [], metrics: [], logs: [] }), clear() {} },
    track: (operation) => operation,
  };
});

afterEach(async () => {
  cleanup();
  owner.abort();
  await client.close();
});

async function openEditor(rows: unknown[] = []) {
  render(<Schedules context={context} />);
  const read = await waitForRequest(transport, "plugins.call");
  await act(async () =>
    respondSuccess(transport, read.id, { output: stringifyJSON({ Rows: rows, NextCursor: "" }) }),
  );
  fireEvent.click(screen.getByRole("button", { name: rows.length ? "Edit" : "Add schedule" }));
  fireEvent.change(screen.getByRole("textbox", { name: "Title" }), {
    target: { value: "Retained draft" },
  });
  return waitForRequest(transport, "providers.list");
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
  expect((await screen.findByRole("alert")).textContent).toContain("failed: internal_error");
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
    expect(screen.getByRole("alert").textContent).toContain("healthy: internal_error"),
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
  const view = render(<Schedules context={captured} />);
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
    "Model choices could not be read: internal_error",
  );
  expect((screen.getByRole("textbox", { name: "Title" }) as HTMLInputElement).value).toBe(
    "Retained draft",
  );
});
