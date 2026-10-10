import type { Model, Provider } from "@flame/runtime-contract/wire";
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
