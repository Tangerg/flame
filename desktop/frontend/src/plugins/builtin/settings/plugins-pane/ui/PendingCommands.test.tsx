import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it } from "vitest";
import {
  createFlameClient,
  createMutationJournal,
  stringifyJSON,
} from "@flame/runtime-contract/client";
import { createMemoryTransport } from "@flame/runtime-contract/client/transports/memory";
import {
  respondSuccess,
  waitForRequest,
} from "@flame/runtime-contract/client/transports/memory.testkit";
import type { CallPluginRequest } from "@flame/runtime-contract/wire";
import type { PackageOperations } from "../application/packages";
import { PendingCommands } from "./PendingCommands";
import { CAPABILITY_JSON } from "../../../../../../../../examples/plugins/reviews/client/methods.generated";

let transport: ReturnType<typeof createMemoryTransport>;
let client: ReturnType<typeof createFlameClient>;
let owner: AbortController;
let operations: PackageOperations;
let key: string;
const records = new Map<string, unknown>();
const installationId = "e4407303-68e3-495d-86d6-9c3f575a51ed";
const original: CallPluginRequest = {
  installationId,
  digest: "a".repeat(64),
  contribution: "review-operations",
  contract: CAPABILITY_JSON,
  method: "io.github.tangerg.flame.examples/reviews.update",
  input: '{"expectedRevision":1,"id":"example-review","status":"resolved"}',
  target: { scope: "installation", reference: installationId },
};

function open() {
  transport = createMemoryTransport();
  client = createFlameClient(transport, {
    mutationJournal: createMutationJournal({
      storage: {
        get: (key) => structuredClone(records.get(key)),
        set: (key, value) => records.set(key, structuredClone(value)),
        remove: (key) => records.delete(key),
        keys: () => [...records.keys()],
      },
      scope: () => ({ namespace: "idp_" + "1".repeat(32), retentionSeconds: 3600 }),
    }),
  });
  owner = new AbortController();
  operations = { ...client.plugins, signal: owner.signal, track: (operation) => operation };
}

beforeEach(async () => {
  records.clear();
  open();
  const observation = Promise.resolve(client.plugins.call(original)).catch(
    (error: unknown) => error,
  );
  await waitForRequest(transport, "plugins.call");
  key = (records.values().next().value as { idempotencyKey: string }).idempotencyKey;
  owner.abort();
  await client.close();
  await observation;
  open();
});

afterEach(async () => {
  cleanup();
  owner.abort();
  await client.close();
});

async function recoverOriginal() {
  fireEvent.click(screen.getByRole("button", { name: "Check original outcome" }));
  const lookup = await waitForRequest(transport, "plugins.getCommand");
  expect(lookup.params).toEqual({ namespace: "idp_" + "1".repeat(32), key });
  await act(async () => respondSuccess(transport, lookup.id, original));
  const request = await waitForRequest(transport, "plugins.call");
  expect(request.params).toEqual(original);
  return request;
}

it("recovers an original command without a catalog row, component or current provider", async () => {
  render(<PendingCommands operations={operations} />);
  expect(screen.getByText(key)).toBeTruthy();
  const request = await recoverOriginal();
  await act(async () =>
    respondSuccess(transport, request.id, {
      output: stringifyJSON({ type: "updated", revision: 2 }),
    }),
  );
  await waitFor(() => expect(records.size).toBe(0));
  expect(screen.getByRole("status").textContent).toContain("Original outcome confirmed");
  expect(screen.getByText('{"type":"updated","revision":2}')).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Check original outcome" })).toBeNull();
  expect(transport.outbox().map((request) => request.method)).toEqual([
    "plugins.getCommand",
    "plugins.call",
  ]);
});

it("retains an unavailable owner's identity without dispatching to another provider", async () => {
  render(<PendingCommands operations={operations} />);
  const request = await recoverOriginal();
  await act(async () =>
    transport.inject({
      jsonrpc: "2.0",
      id: request.id,
      error: { code: -32045, message: "plugin_unavailable", data: { type: "plugin_unavailable" } },
    }),
  );
  await waitFor(() => expect(screen.getByRole("status").textContent).toContain("still unknown"));
  expect(records.size).toBe(1);
  expect(screen.getByText(key)).toBeTruthy();
  expect(screen.getByRole("button", { name: "Check original outcome" })).toBeTruthy();
  expect(transport.outbox().map((request) => request.method)).toEqual([
    "plugins.getCommand",
    "plugins.call",
  ]);
});

it("reports a known refusal separately from an unknown outcome", async () => {
  render(<PendingCommands operations={operations} />);
  const request = await recoverOriginal();
  await act(async () =>
    respondSuccess(transport, request.id, {
      failure: { code: "revisionConflict", message: "the original revision was refused" },
    }),
  );
  await waitFor(() => expect(records.size).toBe(0));
  expect(screen.getByRole("status").textContent).toContain("was refused");
  expect(screen.getByText("the original revision was refused")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Check original outcome" })).toBeNull();
});

it("reports unreadable journal evidence without claiming the list is empty", () => {
  render(
    <PendingCommands
      operations={{
        ...operations,
        pendingCommands() {
          throw new Error("journal evidence is unavailable");
        },
      }}
    />,
  );
  expect(screen.getByText("journal evidence is unavailable")).toBeTruthy();
  expect(screen.queryByText("No unconfirmed plugin commands.")).toBeNull();
  expect(records.size).toBe(1);
  expect(transport.outbox()).toEqual([]);
});
