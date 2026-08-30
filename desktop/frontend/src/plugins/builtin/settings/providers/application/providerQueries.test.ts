import { describe, expect, it } from "vitest";
import { ProviderConfiguration, providerRoleIsAvailable } from "./providerQueries";

describe("providerRoleIsAvailable", () => {
  const providers = [
    ProviderConfiguration.restore({
      id: "configured",
      credential: { masked: "sk****42", source: "stored" },
      configured: true,
      credentialRequirement: "apiKeyRequired",
    }),
    ProviderConfiguration.restore({
      id: "missing-key",
      configured: false,
      credentialRequirement: "apiKeyRequired",
    }),
    ProviderConfiguration.restore({
      id: "ollama",
      configured: true,
      credentialRequirement: "apiKeyOptional",
    }),
  ];

  it("requires both a complete role and a currently configured provider", () => {
    expect(providerRoleIsAvailable({ provider: "configured", model: "model-1" }, providers)).toBe(
      true,
    );
    expect(providerRoleIsAvailable({ provider: "missing-key", model: "model-1" }, providers)).toBe(
      false,
    );
    expect(providerRoleIsAvailable({ provider: "configured" }, providers)).toBe(false);
    expect(providerRoleIsAvailable(undefined, providers)).toBe(false);
    expect(providerRoleIsAvailable({ provider: "ollama", model: "local" }, providers)).toBe(true);
  });

  it("does not treat a role for an absent provider as executable", () => {
    expect(providerRoleIsAvailable({ provider: "removed", model: "model-1" }, providers)).toBe(
      false,
    );
  });
});
