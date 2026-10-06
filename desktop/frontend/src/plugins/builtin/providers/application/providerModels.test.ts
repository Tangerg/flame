import { describe, expect, it } from "vitest";
import { ProviderConfiguration } from "./providerModels";

describe("ProviderConfiguration", () => {
  it("accepts a configured optional-API-key provider without inventing a credential", () => {
    const provider = ProviderConfiguration.restore({
      id: "test-endpoint",
      configured: true,
      credentialRequirement: "apiKeyOptional",
      embeddingCapable: true,
      defaultEmbeddingModel: "nomic-embed-text",
    });

    expect(provider.configured).toBe(true);
    expect(provider.credential).toBeUndefined();
    expect(provider.authentication.requiresAPIKey).toBe(false);
  });
});
