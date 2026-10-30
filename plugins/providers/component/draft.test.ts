import { describe, expect, it } from "vitest";
import { ProviderCredentialsDraft } from "./draft";
describe("ProviderCredentialsDraft", () => {
  it("projects current settings without copying them into an editable draft", () => {
    const draft = ProviderCredentialsDraft.initial();
    expect(draft.apiKey).toBe("");
    expect(draft.baseURL({ baseUrl: "https://api.example.test" })).toBe(
      "https://api.example.test",
    );
  });

  it("owns dirty and endpoint-validity policy", () => {
    const provider = {
      baseUrl: "https://api.example.test",
      requiresBaseUrl: true,
    };
    const initial = ProviderCredentialsDraft.initial();
    expect(initial.dirty(provider)).toBe(false);
    expect(initial.withAPIKey(" key ").dirty(provider)).toBe(true);
    expect(initial.withBaseURL("   ").valid(provider)).toBe(false);
    expect(
      initial.withBaseURL(" https://models.example.test/v1 ").valid(provider),
    ).toBe(true);
  });

  it("preserves the exact secret while normalizing an endpoint", () => {
    const update = ProviderCredentialsDraft.initial()
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
    expect(
      ProviderCredentialsDraft.initial().withBaseURL("").toUpdate(provider),
    ).toEqual({
      provider: "openai",
      baseUrl: { type: "clear" },
    });
  });

  it("settles only the fields that still hold what was submitted", () => {
    const submitted = ProviderCredentialsDraft.initial()
      .withAPIKey("k1")
      .withBaseURL("https://a");
    const typedOn = submitted.withBaseURL("https://b");
    const settled = typedOn.settle(submitted);
    expect(settled.apiKey).toBe("");
    expect(settled.baseURL({ baseUrl: "https://a" })).toBe("https://b");
    expect(
      submitted.settle(submitted).baseURL({ baseUrl: "https://canonical" }),
    ).toBe("https://canonical");
  });

  it("never turns a background endpoint change into an unrequested write", () => {
    const draft =
      ProviderCredentialsDraft.initial().withAPIKey("replacement-key");
    const current = {
      id: "openai",
      baseUrl: "https://changed-by-runtime.example.test",
    };
    expect(draft.baseURL(current)).toBe(current.baseUrl);
    expect(draft.toUpdate(current)).toEqual({
      provider: "openai",
      apiKey: { type: "set", value: "replacement-key" },
    });
  });

  it("retains an endpoint explicitly edited again during a save", () => {
    const submitted =
      ProviderCredentialsDraft.initial().withBaseURL("https://entered");
    const later = submitted.withBaseURL("https://entered");
    expect(
      later.settle(submitted).baseURL({ baseUrl: "https://canonical" }),
    ).toBe("https://entered");
  });
});
