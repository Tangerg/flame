import { describe, expect, it } from "vitest";
import { RpcError, asRunId, asSessionId } from "@flame/runtime-client";
import { type RunEvent } from "@flame/runtime-client/wire";
import { createProviderGate } from "./fakeProvider";
import { collectRunEvents, originalOpeningEvents, within } from "./observation";
import { useRuntimeFixture } from "./fixture";

describe("Go Runtime ↔ HTTP ↔ TypeScript SDK: providers", () => {
  const fixture = useRuntimeFixture("providers");

  it("applies provider credentials and model roles through the live resolvers", async () => {
    const providerId = "openai-compatible";
    await expect(fixture.client.providers.list()).resolves.toMatchObject({
      data: expect.arrayContaining([
        expect.objectContaining({
          id: providerId,
          requiresBaseUrl: true,
        }),
      ]),
    });
    await expect(fixture.client.models.list(providerId)).resolves.toMatchObject({
      data: [expect.objectContaining({ id: "e2e-model", provider: providerId })],
    });
    await expect(fixture.client.providers.test(providerId)).resolves.toEqual({
      outcome: "reachable",
    });

    const updated = await fixture.client.providers.update({
      provider: providerId,
      apiKey: { type: "set", value: "alpha-credential-omega" },
    });
    expect(updated).toMatchObject({
      id: providerId,
      credential: { masked: "al****ga", source: "stored" },
    });
    expect(updated.credential?.masked).not.toContain("alpha-credential-omega");
    await expect(fixture.client.providers.test(providerId)).resolves.toEqual({
      outcome: "reachable",
    });

    await expect(
      fixture.client.models.setUtilityRole({ provider: providerId, model: "e2e-model" }),
    ).resolves.toEqual({ provider: providerId, model: "e2e-model" });
    await expect(fixture.client.models.getUtilityRole()).resolves.toEqual({
      provider: providerId,
      model: "e2e-model",
    });
    await expect(
      fixture.client.models.setEmbeddingRole({ provider: providerId, model: "e2e-model" }),
    ).rejects.toSatisfy(
      (error: unknown) => error instanceof RpcError && error.data.type === "invalid_params",
    );
    await fixture.client.providers.update({
      provider: "openai",
      apiKey: { type: "set", value: "embedding-test-key" },
      baseUrl: { type: "set", value: fixture.providerBaseUrl },
    });
    await expect(
      fixture.client.models.setEmbeddingRole({ provider: "openai", model: "e2e-embedding" }),
    ).resolves.toEqual({ provider: "openai", model: "e2e-embedding" });
    await expect(fixture.client.models.getEmbeddingRole()).resolves.toEqual({
      provider: "openai",
      model: "e2e-embedding",
    });

    await fixture.client.providers.update({
      provider: "openai",
      apiKey: { type: "clear" },
    });
    await expect(fixture.client.models.getEmbeddingRole()).resolves.toEqual({
      provider: "openai",
      model: "e2e-embedding",
    });
    const providersAfterClear = await fixture.client.providers.list();
    expect(
      providersAfterClear.data.find((provider) => provider.id === "openai")?.credential,
    ).toBeUndefined();
    await fixture.client.providers.update({
      provider: "openai",
      apiKey: { type: "set", value: "embedding-test-key" },
      baseUrl: { type: "set", value: fixture.providerBaseUrl },
    });

    await expect(fixture.client.models.setUtilityRole({})).resolves.toEqual({});
    await expect(fixture.client.models.setEmbeddingRole({})).resolves.toEqual({});
    await expect(fixture.client.models.getUtilityRole()).resolves.toEqual({});
    await expect(fixture.client.models.getEmbeddingRole()).resolves.toEqual({});
  }, 30_000);

  it("carries catalog reasoning capabilities and an exact effort into provider execution", async () => {
    const openAIModels = await fixture.client.models.list("openai");
    expect(openAIModels.data).toContainEqual(
      expect.objectContaining({
        id: "gpt-5.6-sol",
        provider: "openai",
        capabilities: expect.objectContaining({
          reasoning: true,
          reasoningLevels: ["none", "low", "medium", "high", "xhigh", "max"],
          reasoningDefaultLevel: "medium",
          inputModalities: expect.arrayContaining(["text", "image"]),
          outputModalities: expect.arrayContaining(["text"]),
        }),
      }),
    );

    const gate = createProviderGate("E2E_REASONING_SELECTION");
    fixture.providerGate = gate;
    try {
      const session = await fixture.client.sessions.create({
        workspace: { path: fixture.root },
        title: "HTTP exact reasoning selection",
      });
      const started = await fixture.client.runs.start({
        sessionId: asSessionId(session.id),
        input: [{ type: "text", text: "E2E_REASONING_SELECTION reach the provider." }],
        provider: "openai-compatible",
        model: "e2e-model",
        reasoningEffort: "high",
      });
      await within(gate.arrived.promise, "the exact reasoning provider request");
      expect(gate.request?.reasoning_effort).toBe("high");
      gate.release.resolve();
      await expect(collectRunEvents(originalOpeningEvents(started))).resolves.toSatisfy(
        (events: RunEvent[]) => events.at(-1)?.event.type === "segment.finished",
      );
      await expect(fixture.client.sessions.get(asSessionId(session.id))).resolves.toMatchObject({
        provider: "openai-compatible",
        model: "e2e-model",
        reasoningEffort: "high",
      });
      await expect(fixture.client.runs.get(asRunId(started.result.runId))).resolves.toMatchObject({
        provider: "openai-compatible",
        model: "e2e-model",
        reasoningEffort: "high",
        status: "finished",
      });
    } finally {
      gate.release.resolve();
      fixture.providerGate = undefined;
    }
  }, 30_000);
});
