import { unavailableComponentNotifications } from "@/test/unavailableNotifications";
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
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { parseJSON, stringifyJSON } from "@qimu/sdk/json";
import { createFlameClient } from "@flame/runtime-client";
import { readFileSync } from "node:fs";
import path from "node:path";
import { createMemoryTransport } from "@flame/runtime-client/testing";
import { respondSuccess, waitForRequest } from "@flame/runtime-client/testing";
import { bindOperations, type ComponentContext } from "@flame/component-sdk";
import { type ResolvePluginPointRequest, type CallPluginRequest } from "@flame/runtime-client/wire";
import { Board } from "../../../../examples/plugins/reviews/component/board";
import {
  CAPABILITY_JSON,
  type MethodInputs,
  type MethodOutputs,
} from "../../../../examples/plugins/reviews/component/methods.generated";
import { ANALYZERS_JSON } from "../../../../examples/plugins/reviews/component/analyzers.generated";
let transport: ReturnType<typeof createMemoryTransport>;

let client: ReturnType<typeof createFlameClient>;

let owner: AbortController;

let context: ComponentContext;

beforeEach(() => {
  transport = createMemoryTransport();
  client = createFlameClient(transport);
  owner = new AbortController();
  context = {
    component: {
      installationId: "e4407303-68e3-495d-86d6-9c3f575a51ed",
      digest: "a".repeat(64),
      componentId: "test",
    },
    saveFile() {
      throw new Error("This fixture must not save files");
    },
    notifications: unavailableComponentNotifications,
    subscribeRootRunSettlements() {
      throw new Error("This fixture must not observe run settlements");
    },
    registerCommand() {
      throw new Error("This fixture must not register client commands");
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
      contribution: "review-operations",
      contract: CAPABILITY_JSON,
    },
    telemetry: {
      read: () => ({ spans: [], metrics: [], logs: [] }),
      clear() {},
    },
    track: (operation) => operation(context.signal),
  };
});

afterEach(async () => {
  cleanup();
  owner.abort();
  await client.close();
});

async function readReview(status: "open" | "resolved", revision = 1) {
  const request = await waitForRequest(transport, "plugins.call");
  await act(async () => {
    respondSuccess(transport, request.id, {
      output: stringifyJSON({
        reviews: [{ id: "example-review", title: "Owned review", status, revision }],
      }),
    });
  });
}

describe("installed Reviews contextual actions", () => {
  it("admits component work before the Runtime client can create a command", () => {
    const refused = new Error("component work capacity exhausted");
    const call = vi.spyOn(client.plugins, "call");
    const operations = bindOperations<MethodInputs, MethodOutputs>(
      {
        ...context,
        track() {
          throw refused;
        },
      },
      CAPABILITY_JSON,
      { scope: "installation", reference: context.binding!.installationId },
    );
    expect(() =>
      operations.command("io.github.tangerg.flame.examples/reviews.update", {
        id: "example-review",
        expectedRevision: 1,
        status: "resolved",
      }),
    ).toThrow(refused);
    expect(call).not.toHaveBeenCalled();
    expect(transport.outbox()).toEqual([]);
  });

  it("retains an original command recovery entrance after the component and client are recreated", async () => {
    const records = new Map<string, unknown>();
    const storage = {
      get: (key: string) => structuredClone(records.get(key)),
      set: (key: string, value: unknown) => records.set(key, structuredClone(value)),
      remove: (key: string) => records.delete(key),
      keys: () => [...records.keys()],
    };
    const openClient = () =>
      createFlameClient(transport, {
        mutationStorage: storage,
        capabilities: () => discovery.capabilities as ServerCapabilities,
      });
    await client.close();
    transport = createMemoryTransport();
    client = openClient();
    context = { ...context, runtime: client };
    const view = testClient.render(<Board context={context} />);
    await readReview("open");
    const point = await waitForRequest(transport, "plugins.resolvePoint");
    await act(async () => respondSuccess(transport, point.id, { data: [] }));
    const save = screen.getByRole("button", { name: "Save status" });
    await waitFor(() => expect((save as HTMLButtonElement).disabled).toBe(false));
    fireEvent.click(save);
    await waitFor(() =>
      expect(
        transport.outbox().filter((request) => request.method === "plugins.call"),
      ).toHaveLength(2),
    );
    expect(records.size).toBe(1);

    const originalRequest = transport
      .outbox()
      .filter((request) => request.method === "plugins.call")[1]!;

    await act(async () => {
      view.unmount();
      owner.abort();
      await client.close();
    });
    expect(records.size).toBe(1);
    transport = createMemoryTransport();
    client = openClient();
    owner = new AbortController();
    context = { ...context, runtime: client, signal: owner.signal };
    testClient.render(<Board context={context} />);
    await readReview("resolved", 2);
    const replacementPoint = await waitForRequest(transport, "plugins.resolvePoint");
    await act(async () => respondSuccess(transport, replacementPoint.id, { data: [] }));

    expect(await screen.findByRole("button", { name: "Check original outcome" })).toBeTruthy();
    expect(
      (screen.getByRole("button", { name: "Save status" }) as HTMLButtonElement).disabled,
    ).toBe(true);
    expect(records.size).toBe(1);
    fireEvent.click(screen.getByRole("button", { name: "Check original outcome" }));
    const lookup = await waitForRequest(transport, "plugins.getCommand");
    const admittedRequest = {
      ...(originalRequest.params as CallPluginRequest),
      input: stringifyJSON(parseJSON((originalRequest.params as CallPluginRequest).input), true),
    };
    await act(async () => respondSuccess(transport, lookup.id, admittedRequest));
    const recovered = await waitFor(() => {
      const requests = transport.outbox().filter((request) => request.method === "plugins.call");
      expect(requests).toHaveLength(2);
      return requests[1]!;
    });
    expect(recovered.params).toEqual(admittedRequest);
    await act(async () =>
      respondSuccess(transport, recovered.id, {
        output: stringifyJSON({
          type: "updated",
          review: { id: "example-review", title: "Owned review", status: "resolved", revision: 2 },
        }),
      }),
    );
    await waitFor(() => expect(records.size).toBe(0));
    expect(screen.queryByRole("button", { name: "Check original outcome" })).toBeNull();
  });

  it("preserves the generated point contract through the public TypeScript boundary", () => {
    const source = readFileSync(
      path.resolve(
        "../../examples/plugins/reviews/package/io.github.tangerg.flame/contracts/analyzers.generated.json",
      ),
      "utf8",
    );
    expect(parseJSON(ANALYZERS_JSON)).toEqual(parseJSON(source));
  });
  it.each([
    { status: "open" as const, contributors: true },
    { status: "open" as const, contributors: false },
    { status: "resolved" as const, contributors: false },
  ])("uses admitted point metadata for $status / contributors=$contributors", async (scenario) => {
    testClient.render(<Board context={context} />);
    await readReview(scenario.status);
    const request = await waitForRequest(transport, "plugins.resolvePoint");
    const params = request.params as ResolvePluginPointRequest;
    expect(params.target).toEqual({
      scope: "installation",
      reference: context.binding!.installationId,
    });
    expect(JSON.parse(params.attributes)).toEqual({ reviewStatus: scenario.status });
    expect(screen.queryByRole("button", { name: /^Analyze$/ })).toBeNull();
    await act(async () => {
      respondSuccess(transport, request.id, {
        data: scenario.contributors
          ? [
              {
                installationId: "46c0f293-cfe6-4071-8870-7a94c59f7db3",
                digest: "b".repeat(64),
                contribution: "unresolved-review",
                value: stringifyJSON({ title: "Unresolved review", contribution: "analyzer" }),
              },
            ]
          : [],
      });
    });
    await waitFor(() => expect(screen.queryByRole("status")).toBeNull());
    expect(screen.queryByRole("button", { name: /^Analyze$/ }) !== null).toBe(
      scenario.contributors,
    );

    const accepted = transport.outbox();
    fireEvent.click(screen.getByRole("combobox", { name: "New status" }));
    await screen.findByRole("listbox");
    expect(transport.outbox()).toEqual(accepted);
  });

  it("keeps the accepted review and explicitly reports failed action resolution", async () => {
    testClient.render(<Board context={context} />);
    await readReview("open");
    const request = await waitForRequest(transport, "plugins.resolvePoint");
    await act(async () => {
      transport.inject({
        jsonrpc: "2.0",
        id: request.id,
        error: { code: -32603, message: "internal_error", data: { type: "internal_error" } },
      });
    });
    expect((await screen.findByRole("alert")).textContent).toContain(
      "Analyzer actions could not be resolved",
    );
    expect(screen.getByText("Owned review")).toBeTruthy();
    expect(screen.queryByRole("button", { name: /^Analyze$/ })).toBeNull();
  });

  it("does not expose the previous context's actions during a replacement read", async () => {
    const view = testClient.render(<Board context={context} />);
    await readReview("open");
    const original = await waitForRequest(transport, "plugins.resolvePoint");
    await act(async () => {
      respondSuccess(transport, original.id, {
        data: [
          {
            installationId: "46c0f293-cfe6-4071-8870-7a94c59f7db3",
            digest: "b".repeat(64),
            contribution: "unresolved-review",
            value: stringifyJSON({ title: "Unresolved review", contribution: "analyzer" }),
          },
        ],
      });
    });
    expect(screen.getByRole("button", { name: /^Analyze$/ })).toBeTruthy();

    view.rerender(<Board context={{ ...context, signal: new AbortController().signal }} />);
    expect(screen.queryByRole("button", { name: /^Analyze$/ })).toBeNull();
    expect(screen.getByRole("status").textContent).toContain("Reading current reviews");
    const replacement = await waitFor(() => {
      const requests = transport.outbox().filter((request) => request.method === "plugins.call");
      expect(requests).toHaveLength(2);
      return requests[1]!;
    });
    await act(async () => {
      respondSuccess(transport, replacement.id, {
        output: stringifyJSON({
          reviews: [
            { id: "example-review", title: "Owned review", status: "resolved", revision: 2 },
          ],
        }),
      });
    });
    const point = await waitFor(() => {
      const requests = transport
        .outbox()
        .filter((request) => request.method === "plugins.resolvePoint");
      expect(requests).toHaveLength(2);
      return requests[1]!;
    });
    await act(async () => respondSuccess(transport, point.id, { data: [] }));
    await waitFor(() => expect(screen.queryByRole("status")).toBeNull());
    expect(screen.queryByRole("button", { name: /^Analyze$/ })).toBeNull();
  });
});
