import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it } from "vitest";
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

async function openEditor() {
  render(<Schedules context={context} />);
  const read = await waitForRequest(transport, "plugins.call");
  await act(async () =>
    respondSuccess(transport, read.id, { output: stringifyJSON({ Rows: [], NextCursor: "" }) }),
  );
  fireEvent.click(screen.getByRole("button", { name: "Add schedule" }));
  fireEvent.change(screen.getByRole("textbox", { name: "Title" }), {
    target: { value: "Retained draft" },
  });
  return waitForRequest(transport, "providers.list");
}

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
