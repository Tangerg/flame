import { stringifyJSON } from "@qimu/sdk/json";
import { expect, it } from "vitest";
import type { RpcCallOptions, RpcClient } from "./client";
import { createMethods } from "./methods";

it("point inspection sends the admitted contract string without re-encoding schema numbers", async () => {
  const contract = `{"protocol":"io.github.tangerg.qimu/portable/1","reference":"example.threshold","composition":"collection","scope":"workspace","optional":true,"ordering":"canonicalKeys","value":{"type":"integer","maximum":18446744073709551615},"applicability":true}`;
  const signal = new AbortController().signal;
  const methods = createMethods({
    call: async (method: string, params: unknown, options?: RpcCallOptions) => {
      expect(method).toBe("plugins.resolvePoint");
      const received = JSON.parse(JSON.stringify(params));
      expect(received.contract).toBe(contract);
      expect(options?.signal).toBe(signal);
      expect(options?.idempotencyKey).toBeUndefined();
      return { data: [] };
    },
  } as unknown as RpcClient);
  await expect(
    methods.plugins.resolvePoint(
      {
        contract,
        target: { scope: "workspace", reference: "/project" },
        attributes: stringifyJSON({}),
        order: [],
      },
      signal,
    ),
  ).resolves.toEqual({ data: [] });
});
