import discovery from "@flame/runtime-client/testing/samples/method.discover.resp.json";
import type { ServerCapabilities } from "@flame/runtime-client/wire";
import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it } from "vitest";
import { stringifyJSON } from "@qimu/sdk/json";
import { createFlameClient } from "@flame/runtime-client";
import { createMemoryTransport } from "@flame/runtime-client/testing";
import { respondSuccess, waitForRequest } from "@flame/runtime-client/testing";
import { type CallPluginRequest } from "@flame/runtime-client/wire";
import { type PackageOperations } from "@/features/plugins/adapters/packages";
import { PendingCommands } from "@/features/plugins/ui/PendingCommands";
import { CAPABILITY_JSON } from "../../../../../../examples/plugins/reviews/component/methods.generated";
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
    mutationStorage: {
      get: (key) => structuredClone(records.get(key)),
      set: (key, value) => records.set(key, structuredClone(value)),
      remove: (key) => records.delete(key),
      keys: () => [...records.keys()],
    },
    capabilities: () => discovery.capabilities as ServerCapabilities,
  });
  owner = new AbortController();
  operations = {
    ...client.plugins,
    signal: owner.signal,
    track: (operation) => operation(owner.signal),
  };
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
  expect(lookup.params).toEqual({
    namespace: discovery.capabilities.limits.idempotency.namespace,
    key,
  });
  await act(async () => respondSuccess(transport, lookup.id, original));
  const request = await waitForRequest(transport, "plugins.call");
  expect(request.params).toEqual(original);
  return request;
}

it("retires never-dispatched work using only its original identity", async () => {
  testClient.render(<PendingCommands operations={operations} />);
  fireEvent.click(screen.getByRole("button", { name: "Retire if never dispatched" }));
  const request = await waitForRequest(transport, "plugins.retireCommand");
  expect(request.params).toEqual({
    namespace: discovery.capabilities.limits.idempotency.namespace,
    key,
  });
  await act(async () => respondSuccess(transport, request.id, {}));
  await waitFor(() => expect(records.size).toBe(0));
  expect(screen.getByRole("status").textContent).toContain("retired before dispatch");
  expect(transport.outbox().map((request) => request.method)).toEqual(["plugins.retireCommand"]);
});

it("keeps dispatched uncertainty after Runtime refuses retirement", async () => {
  testClient.render(<PendingCommands operations={operations} />);
  fireEvent.click(screen.getByRole("button", { name: "Retire if never dispatched" }));
  const request = await waitForRequest(transport, "plugins.retireCommand");
  await act(async () =>
    transport.inject({
      jsonrpc: "2.0",
      id: request.id,
      error: { code: -32603, message: "internal_error", data: { type: "internal_error" } },
    }),
  );
  await waitFor(() => expect(screen.getByRole("status").textContent).toContain("still unknown"));
  expect(records.size).toBe(1);
  expect(screen.getByRole("button", { name: "Check original outcome" })).toBeTruthy();
});

it("recovers an original command without a catalog row, component or current provider", async () => {
  testClient.render(<PendingCommands operations={operations} />);
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
  testClient.render(<PendingCommands operations={operations} />);
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
  testClient.render(<PendingCommands operations={operations} />);
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
  testClient.render(
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
