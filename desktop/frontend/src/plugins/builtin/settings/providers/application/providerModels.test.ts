import { describe, expect, it } from "vitest";
import { ProviderConfiguration } from "./providerModels";

describe("ProviderConfiguration", () => {
  it("accepts a configured optional-API-key provider without inventing a credential", () => {
    const provider = ProviderConfiguration.restore({
      id: "ollama",
      configured: true,
      credentialRequirement: "apiKeyOptional",
      embeddingCapable: true,
      defaultEmbeddingModel: "nomic-embed-text",
    });

    expect(provider.configured).toBe(true);
    expect(provider.credential).toBeUndefined();
    expect(provider.authentication.requiresAPIKey).toBe(false);
  });

  it("rejects configured state that violates the published provider policy", () => {
    expect(() =>
      ProviderConfiguration.restore({
        id: "openai",
        configured: true,
        credentialRequirement: "apiKeyRequired",
      }),
    ).toThrow("required API key");
    expect(() =>
      ProviderConfiguration.restore({
        id: "compatible",
        configured: true,
        credentialRequirement: "apiKeyOptional",
        requiresBaseUrl: true,
      }),
    ).toThrow("required base URL");
    expect(() =>
      ProviderConfiguration.restore({
        id: "anthropic",
        configured: false,
        credentialRequirement: "apiKeyRequired",
        defaultEmbeddingModel: "impossible",
      }),
    ).toThrow("without embeddings");
  });
});
