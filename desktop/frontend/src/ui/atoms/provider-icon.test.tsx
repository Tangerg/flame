import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { ProviderIcon, providerDisplayName } from "./provider-icon";

// The Runtime's provider ids, which are the only strings that reach this atom. The wire types
// `provider` as an open string, so nothing else can hold this table to them.
const RUNTIME_PROVIDER_IDS = [
  "alibaba",
  "anthropic",
  "azureopenai",
  "deepseek",
  "fireworks",
  "google",
  "groq",
  "huggingface",
  "minimax",
  "mistral",
  "moonshot",
  "ollama",
  "openai",
  "openrouter",
  "perplexity",
  "together",
  "xai",
  "xiaomi",
  "zhipu",
];

afterEach(cleanup);

function hasBrandMark(provider: string): boolean {
  const { container } = render(<ProviderIcon provider={provider} />);
  return container.querySelector("svg[data-icon-name]") === null;
}

describe("ProviderIcon", () => {
  it("carries a brand mark for every provider the Runtime can be configured with", () => {
    const generic = RUNTIME_PROVIDER_IDS.filter((id) => !hasBrandMark(id));

    expect(generic).toEqual(["xiaomi"]);
  });

  it("names the brand rather than capitalising the Runtime's own spelling", () => {
    expect(providerDisplayName("azureopenai")).toBe("Azure OpenAI");
    expect(providerDisplayName("xai")).toBe("xAI");
  });

  it("falls back for a provider it does not know, without dropping the name", () => {
    expect(providerDisplayName("acme")).toBe("Acme");
    expect(hasBrandMark("acme")).toBe(false);
  });
});
