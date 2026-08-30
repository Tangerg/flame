import { describe, expect, it } from "vitest";
import { ProviderCredentialsDraft } from "./providerDraft";

describe("ProviderCredentialsDraft", () => {
  it("initializes from persisted settings without copying secrets", () => {
    expect(ProviderCredentialsDraft.initial({ baseUrl: "https://api.example.test" })).toMatchObject(
      {
        apiKey: "",
        baseUrl: "https://api.example.test",
      },
    );
  });

  it("owns dirty and endpoint-validity policy", () => {
    const provider = { baseUrl: "https://api.example.test", requiresBaseUrl: true };
    const initial = ProviderCredentialsDraft.initial(provider);
    expect(initial.dirty(provider)).toBe(false);
    expect(initial.withAPIKey(" key ").dirty(provider)).toBe(true);
    expect(initial.withBaseURL("   ").valid(provider)).toBe(false);
    expect(initial.withBaseURL(" https://models.example.test/v1 ").valid(provider)).toBe(true);
  });

  it("preserves the exact secret while normalizing an endpoint", () => {
    const update = ProviderCredentialsDraft.initial({})
      .withAPIKey(" sk-test ")
      .withBaseURL(" https://gateway.example.test ")
      .toUpdate({ id: "openai" });
    expect(update).toEqual({
      provider: "openai",
      apiKey: { type: "set", value: " sk-test " },
      baseUrl: { type: "set", value: "https://gateway.example.test" },
    });
  });

  it("preserves an untouched secret and explicitly clears an edited endpoint", () => {
    const provider = { id: "openai", baseUrl: "https://gateway.example.test" };
    expect(ProviderCredentialsDraft.initial(provider).withBaseURL("").toUpdate(provider)).toEqual({
      provider: "openai",
      baseUrl: { type: "clear" },
    });
  });
});
