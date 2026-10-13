import { expect, it } from "vitest";
import type { RpcCallOptions, RpcClient } from "./client";
import { RpcTransportError } from "./errors";
import { createMethods } from "./methods";

it("provider selection preserves inspected identity and the original command after lost acknowledgement", async () => {
  const selection = {
    role: "memory",
    installationId: "940ac827-b431-455b-af4b-e3a170bcfda0",
    contribution: "memory-provider",
  };
  const original = structuredClone(selection);
  const attempts: { params: unknown; key: string | undefined }[] = [];
  let lost = true;
  const methods = createMethods({
    call: async (method: string, params: unknown, options?: RpcCallOptions) => {
      if (method === "plugins.listProviders") {
        expect(params).toEqual({});
        expect(options?.idempotencyKey).toBeUndefined();
        return { data: [original] };
      }
      expect(method).toBe("plugins.selectProvider");
      attempts.push({ params: structuredClone(params), key: options?.idempotencyKey });
      if (lost) throw new RpcTransportError("acknowledgement lost");
      return original;
    },
  } as unknown as RpcClient);
  await expect(methods.plugins.listProviders()).resolves.toEqual({ data: [original] });
  const command = methods.plugins.selectProvider(selection);
  selection.contribution = "replacement";
  selection.role = "schedules";
  await expect(command).rejects.toBeInstanceOf(RpcTransportError);
  lost = false;
  await expect(command.retry()).resolves.toEqual(original);
  expect(attempts.length).toBeGreaterThan(1);
  for (const attempt of attempts)
    expect(attempt).toEqual({ params: original, key: command.idempotencyKey });
});
