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
  const providerReads: { params: unknown; key: string | undefined }[] = [];
  let lost = true;
  const methods = createMethods({
    call: async (method: string, params: unknown, options?: RpcCallOptions) => {
      if (method === "plugins.listProviders") {
        providerReads.push({ params: structuredClone(params), key: options?.idempotencyKey });
        return { data: [original] };
      }
      expect(method).toBe("plugins.selectProvider");
      attempts.push({ params: structuredClone(params), key: options?.idempotencyKey });
      if (lost) throw new RpcTransportError("acknowledgement lost");
      return original;
    },
  } as unknown as RpcClient);
  await expect(methods.plugins.listProviders()).resolves.toEqual({ data: [original] });
  expect(providerReads).toEqual([{ params: {}, key: undefined }]);
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

it("composition changes preserve original ordered members and command identity after response loss", async () => {
  const input = {
    contract: "consumer-contract",
    members: [
      { installationId: "940ac827-b431-455b-af4b-e3a170bcfda0", contribution: "outer" },
      { installationId: "9a842011-9fc0-4f36-9a8d-83e323213192", contribution: "inner" },
    ],
  };
  const original = structuredClone(input);
  const attempts: { params: unknown; key: string | undefined }[] = [];
  const resolutions: { params: unknown; key: string | undefined }[] = [];
  let lost = true;
  const methods = createMethods({
    call: async (method: string, params: unknown, options?: RpcCallOptions) => {
      if (method === "plugins.listCompositions") return { data: [original] };
      if (method === "plugins.resolveComposition") {
        resolutions.push({ params: structuredClone(params), key: options?.idempotencyKey });
        return { data: [] };
      }
      expect(method).toBe("plugins.setComposition");
      attempts.push({ params: structuredClone(params), key: options?.idempotencyKey });
      if (lost) throw new RpcTransportError("acknowledgement lost");
      return original;
    },
  } as unknown as RpcClient);
  const command = methods.plugins.setComposition(input);
  input.members.reverse();
  await expect(command).rejects.toBeInstanceOf(RpcTransportError);
  lost = false;
  await expect(command.retry()).resolves.toEqual(original);
  expect(attempts).toHaveLength(3);
  for (const attempt of attempts)
    expect(attempt).toEqual({ params: original, key: command.idempotencyKey });
  await expect(methods.plugins.listCompositions()).resolves.toEqual({ data: [original] });
  await expect(
    methods.plugins.resolveComposition({ contract: original.contract }),
  ).resolves.toEqual({ data: [] });
  expect(resolutions).toEqual([{ params: { contract: original.contract }, key: undefined }]);
});
