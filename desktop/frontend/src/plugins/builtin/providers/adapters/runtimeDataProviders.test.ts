import { describe, expect, it, vi } from "vitest";
import { lookupDataProvider } from "@/plugins/sdk/selectors";
import { createFlameClient, JSONRPC_VERSION } from "@flame/runtime-contract/client";
import { createMemoryTransport } from "@flame/runtime-contract/client/transports/memory";
import {
  respondSuccess,
  waitForRequest,
} from "@flame/runtime-contract/client/transports/memory.testkit";
import type { WireMethodName } from "@flame/runtime-contract/methods";
import { PROBLEM_CODES } from "@flame/runtime-contract/wire";
import { contributeForTest } from "@/plugins/sdk/testKernel";
import { SelectableModel } from "../application/selectableModel";
import { registerProviderDataProviders } from "./runtimeDataProviders";

async function runProvider<T>(
  key: string,
  responses: Array<[method: WireMethodName, result: unknown]>,
  params?: unknown,
): Promise<{ value: T; requests: Array<{ method: string; params: unknown }> }> {
  const t = createMemoryTransport();
  const client = createFlameClient(t);
  try {
    await contributeForTest((ctx) => registerProviderDataProviders(ctx, () => client));

    const fetcher = lookupDataProvider<T>(key);
    if (!fetcher) throw new Error(`no provider for "${key}"`);
    const pending = fetcher(params);
    const requests: Array<{ method: string; params: unknown }> = [];
    for (const [method, result] of responses) {
      const req = await waitForRequest(t, method);
      requests.push({ method: req.method, params: req.params });
      respondSuccess(t, req.id, result);
    }
    return { value: await pending, requests };
  } finally {
    await client.close();
  }
}

describe("providers Runtime data providers", () => {
  it("models: queries configured providers, including optional-auth endpoints", async () => {
    const { value, requests } = await runProvider<SelectableModel[]>("models", [
      [
        "providers.list",
        {
          data: [
            {
              id: "disabled",
              configured: false,
              credentialRequirement: "apiKeyRequired",
            },
            {
              id: "test-endpoint",
              configured: true,
              credentialRequirement: "apiKeyOptional",
            },
          ],
        },
      ],
      [
        "models.list",
        {
          data: [
            {
              id: "llama-test",
              provider: "test-endpoint",
              displayName: "Llama Test",
              tokenLimits: {
                contextWindow: 258_000,
                maxInputTokens: 250_000,
                maxOutputTokens: 32_000,
              },
              capabilities: {
                reasoning: true,
                reasoningLevels: ["low", "medium", "high"],
                reasoningDefaultLevel: "medium",
                multimodal: true,
                inputModalities: ["text", "image"],
                outputModalities: ["text"],
                toolUse: true,
                structuredOutput: true,
              },
            },
          ],
        },
      ],
    ]);

    expect(requests).toEqual([
      { method: "providers.list", params: {} },
      { method: "models.list", params: { provider: "test-endpoint" } },
    ]);
    expect(value).toHaveLength(1);
    expect(value[0]).toBeInstanceOf(SelectableModel);
    expect(value[0]).toMatchObject({
      id: "llama-test",
      provider: "test-endpoint",
      label: "Llama Test",
      tokenLimits: {
        contextWindow: 258_000,
        maxInputTokens: 250_000,
        maxOutputTokens: 32_000,
      },
      reasoning: true,
      reasoningLevels: ["low", "medium", "high"],
      reasoningDefaultLevel: "medium",
      inputModalities: ["text", "image"],
      outputModalities: ["text"],
      toolUse: true,
      structuredOutput: true,
    });
    expect(value[0]?.acceptsInput("image")).toBe(true);
    expect(value[0]?.reasoningLevelOrDefault("unsupported")).toBe("medium");
  });

  it("keeps one multi-stage provider read on its admitted client generation", async () => {
    const retiredTransport = createMemoryTransport();
    const retiredClient = createFlameClient(retiredTransport);
    const successorTransport = createMemoryTransport();
    const successorClient = createFlameClient(successorTransport);
    try {
      let activeClient = retiredClient;
      await contributeForTest((ctx) => registerProviderDataProviders(ctx, () => activeClient));
      const fetcher = lookupDataProvider("models");
      if (!fetcher) throw new Error('no provider for "models"');

      const pending = fetcher();
      const providersRequest = await waitForRequest(retiredTransport, "providers.list");
      activeClient = successorClient;
      respondSuccess(retiredTransport, providersRequest.id, {
        data: [
          {
            id: "openai",
            configured: true,
            credentialRequirement: "apiKeyRequired",
            credential: { masked: "sk****42", source: "stored" },
          },
        ],
      });

      await vi.waitFor(() => {
        expect(
          [...retiredTransport.outbox(), ...successorTransport.outbox()].some(
            ({ method }) => method === "models.list",
          ),
        ).toBe(true);
      });
      const retiredModelsRequest = retiredTransport
        .outbox()
        .find(({ method }) => method === "models.list");
      const successorModelsRequest = successorTransport
        .outbox()
        .find(({ method }) => method === "models.list");
      const actualTransport = retiredModelsRequest ? retiredTransport : successorTransport;
      const actualRequest = retiredModelsRequest ?? successorModelsRequest;
      if (!actualRequest) throw new Error("models.list was not requested");
      respondSuccess(actualTransport, actualRequest.id, { data: [] });
      await pending;

      expect(retiredModelsRequest).toBeDefined();
      expect(successorModelsRequest).toBeUndefined();

      const successorPending = fetcher();
      const successorProvidersRequest = await waitForRequest(successorTransport, "providers.list");
      respondSuccess(successorTransport, successorProvidersRequest.id, { data: [] });
      await successorPending;
      expect(
        retiredTransport.outbox().filter(({ method }) => method === "providers.list"),
      ).toHaveLength(1);
      expect(
        successorTransport.outbox().filter(({ method }) => method === "providers.list"),
      ).toHaveLength(1);
    } finally {
      await Promise.all([retiredClient.close(), successorClient.close()]);
    }
  });

  it("models: preserves a Runtime failure instead of presenting an empty catalog", async () => {
    const transport = createMemoryTransport();
    const client = createFlameClient(transport);
    await contributeForTest((ctx) => registerProviderDataProviders(ctx, () => client));
    const fetcher = lookupDataProvider("models");
    if (!fetcher) throw new Error('no provider for "models"');

    const pending = fetcher();
    const providersRequest = await waitForRequest(transport, "providers.list");
    respondSuccess(transport, providersRequest.id, {
      data: [
        {
          id: "openai",
          configured: true,
          credentialRequirement: "apiKeyRequired",
          credential: { masked: "sk****42", source: "stored" },
        },
      ],
    });
    const modelsRequest = await waitForRequest(transport, "models.list");
    transport.inject({
      jsonrpc: JSONRPC_VERSION,
      id: modelsRequest.id,
      error: {
        code: PROBLEM_CODES.internal_error,
        message: "internal_error",
        data: { type: "internal_error" },
      },
    });

    await expect(pending).rejects.toMatchObject({ name: "RpcError" });
    await client.close();
  });
});
