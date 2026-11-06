import {
  type EmbeddingRole,
  type Model,
  type Provider,
  type UtilityRole,
} from "@flame/runtime-client/wire";
import { VISUAL_PRIMARY_MODEL_CONTEXT_WINDOW } from "./agentFixtureFacts";
export const VISUAL_MODELS: Model[] = [
  {
    id: "gpt-5.6-sol",
    provider: "openai",
    displayName: "GPT-5.6 Sol",
    default: true,
    capabilities: {
      inputModalities: ["text", "image", "pdf"],
      outputModalities: ["text"],
      reasoning: true,
      reasoningLevels: ["none", "low", "medium", "high", "xhigh", "max"],
      reasoningDefaultLevel: "medium",
      toolUse: true,
      structuredOutput: true,
    },
    tokenLimits: {
      contextWindow: VISUAL_PRIMARY_MODEL_CONTEXT_WINDOW,
      maxInputTokens: 922_000,
      maxOutputTokens: 128_000,
    },
    knowledgeCutoff: "2026-02-16T00:00:00Z",
  },
  {
    id: "qwen-mt-plus",
    provider: "alibaba",
    displayName: "Qwen MT Plus",
    capabilities: { inputModalities: ["text"], outputModalities: ["text"] },
    tokenLimits: { contextWindow: 32_768 },
  },
];

export const VISUAL_PROVIDERS: Provider[] = [
  {
    id: "openai",
    baseUrl: "https://api.openai.com/v1",
    credential: { masked: "sk-…7F2A", source: "stored" },
    credentialRequirement: "apiKeyRequired",
    configured: true,
    embeddingCapable: true,
    defaultEmbeddingModel: "text-embedding-3-large",
  },
  {
    id: "anthropic",
    baseUrl: "https://api.anthropic.com",
    credentialRequirement: "apiKeyRequired",
    configured: false,
    embeddingCapable: false,
  },
];

export interface VisualModelFacts {
  providers: Provider[];
  models: Model[];
  utility: UtilityRole;
  embedding: EmbeddingRole;
}

export const VISUAL_MODEL_FACTS: VisualModelFacts = {
  providers: VISUAL_PROVIDERS,
  models: VISUAL_MODELS,
  utility: { provider: VISUAL_MODELS[0]!.provider, model: VISUAL_MODELS[0]!.id },
  embedding: {},
};

export const LONG_MODEL_NAME =
  "Research model for detailed workspace analysis with extended context";
export const LONG_MODEL_ID = "workspace-analysis-2026-10-extended-context";
export const LONG_MODEL_FACTS: VisualModelFacts = {
  providers: VISUAL_PROVIDERS.map((provider) => ({
    ...provider,
    configured: true,
    embeddingCapable: true,
    defaultEmbeddingModel: `${provider.id}-embedding-default`,
  })),
  models: [
    { provider: "openai", id: LONG_MODEL_ID, displayName: LONG_MODEL_NAME },
    { provider: "openai", id: "research-one", displayName: "Research model" },
    { provider: "anthropic", id: "research-two", displayName: "Research model" },
  ],
  utility: { provider: "openai", model: LONG_MODEL_ID },
  embedding: { provider: "openai", model: "custom-embedding" },
};
