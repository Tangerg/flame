import { describe, expect, it, vi } from "vitest";
import { validateMethodParams, validateWire } from "@flame/runtime-contract/validate";
import { createRpcClient } from "./client";
import { RpcProtocolError } from "./errors";
import { createFlameClient } from "./sdk";
import { createMemoryTransport } from "./transports/memory";

const target = {
  installationId: "940ac827-b431-455b-af4b-e3a170bcfda0",
  digest: "1".repeat(64),
};
const configuration = { ...target, serverChanges: {}, skillChanges: {}, valueChanges: {} };

describe("generated request validation", () => {
  it("refuses invalid requests before request identity publication or transport send", async () => {
    const transport = createMemoryTransport();
    const client = createRpcClient(transport);
    const onRequestRpcId = vi.fn();
    const params = { ...configuration, unexpected: true };
    await expect(
      client.call("plugins.configure", params, { onRequestRpcId }),
    ).rejects.toBeInstanceOf(RpcProtocolError);
    expect(onRequestRpcId).not.toHaveBeenCalled();
    expect(transport.outbox()).toEqual([]);
    await client.close();
  });

  it("refuses invalid mutations before reserving a durable replay identity", async () => {
    const transport = createMemoryTransport();
    const reserve = vi.fn(() => undefined);
    const client = createFlameClient(transport, { mutationJournal: { reserve, dispose: vi.fn() } });
    const params = { ...configuration, unexpected: true };
    await expect(client.plugins.configure(params)).rejects.toBeInstanceOf(RpcProtocolError);
    expect(reserve).not.toHaveBeenCalled();
    expect(transport.outbox()).toEqual([]);
    await client.close();
  });

  it.each(["call", "default"] as const)(
    "refuses invalid %s metadata before request identity publication or transport send",
    async (source) => {
      const transport = createMemoryTransport();
      const send = vi.spyOn(transport, "send").mockRejectedValue(new Error("unexpected dispatch"));
      const requestMeta = { clientInfo: { name: "editor", version: "1", unexpected: true } };
      const client = createRpcClient(
        transport,
        source === "default" ? { requestMeta: () => requestMeta } : {},
      );
      const onRequestRpcId = vi.fn();
      await expect(
        client.call(
          "plugins.list",
          {},
          {
            onRequestRpcId,
            ...(source === "call" ? { requestMeta } : {}),
          },
        ),
      ).rejects.toBeInstanceOf(RpcProtocolError);
      expect(onRequestRpcId).not.toHaveBeenCalled();
      expect(send).not.toHaveBeenCalled();
      expect(transport.outbox()).toEqual([]);
      await client.close();
    },
  );

  it.each([
    { ...configuration, unexpected: true },
    { ...configuration, valueChanges: { token: { type: "set", value: "", unexpected: true } } },
  ])("refuses unknown members throughout typed requests", (params) => {
    expect(validateMethodParams("plugins.configure", params)).toEqual([
      {
        path: Object.hasOwn(params, "unexpected")
          ? "plugins.configure.params.unexpected"
          : 'plugins.configure.params.valueChanges["token"].unexpected',
        detail: "must not be present",
      },
    ]);
  });

  it("refuses unknown members in request arrays and empty requests", () => {
    expect(
      validateMethodParams("plugins.configure", {
        ...configuration,
        serverChanges: { backend: "disable" },
        skillChanges: { review: "toggle" },
      }),
    ).toEqual([
      { path: 'plugins.configure.params.skillChanges["review"]', detail: expect.any(String) },
    ]);
    expect(
      validateMethodParams("runs.resume", {
        runId: "run_1",
        responses: [],
        input: [{ type: "text", text: "continue", unexpected: true }],
      }),
    ).toEqual([{ path: "runs.resume.params.input[0].unexpected", detail: "must not be present" }]);
    expect(validateMethodParams("plugins.list", { unexpected: true })).toEqual([
      { path: "plugins.list.params.unexpected", detail: "must not be present" },
    ]);
  });

  it("distinguishes literal map keys from nested properties", () => {
    expect(
      validateMethodParams("plugins.configure", {
        ...configuration,
        valueChanges: { "token.name": { type: "set", value: null } },
      }),
    ).toContainEqual({
      path: 'plugins.configure.params.valueChanges["token.name"].value',
      detail: "expected a string",
    });
  });

  it("preserves declared maps, opaque tool inputs, and result growth", () => {
    expect(
      validateMethodParams("plugins.configure", {
        ...configuration,
        valueChanges: { token: { type: "set", value: "" }, obsolete: { type: "clear" } },
      }),
    ).toEqual([]);
    expect(
      validateMethodParams("tools.invoke", {
        name: "inspect",
        arguments: { type: "unrelated", nested: { unexpected: null } },
      }),
    ).toEqual([]);
    expect(validateWire("PluginValueChange", { type: "clear", futureField: true })).toEqual([]);
  });
});
