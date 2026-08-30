import {
  validateModelIdentity,
  validateProviderIdentity,
  validateReasoningEffortIdentity,
} from "./modelIdentity";

/**
 * The Desktop application's immutable view of provider-published model token
 * facts. Construction rejects empty objects and numeric sentinels so consumers
 * can use property presence directly without truthiness-based policy.
 */
export class SelectableModelTokenLimits {
  readonly contextWindow?: number;
  readonly maxInputTokens?: number;
  readonly maxOutputTokens?: number;

  constructor(value: {
    contextWindow?: number;
    maxInputTokens?: number;
    maxOutputTokens?: number;
  }) {
    const facts = [value.contextWindow, value.maxInputTokens, value.maxOutputTokens];
    if (facts.every((fact) => fact === undefined)) {
      throw new Error("model token limits require at least one published fact");
    }
    for (const fact of facts) {
      if (fact !== undefined && (!Number.isSafeInteger(fact) || fact <= 0)) {
        throw new Error("model token limits must be positive safe integers");
      }
    }
    if (
      value.contextWindow !== undefined &&
      value.maxInputTokens !== undefined &&
      value.maxInputTokens > value.contextWindow
    ) {
      throw new Error("model max input tokens exceed its context window");
    }
    this.contextWindow = value.contextWindow;
    this.maxInputTokens = value.maxInputTokens;
    this.maxOutputTokens = value.maxOutputTokens;
    Object.freeze(this);
  }
}

/**
 * The model-picker projection kept by Desktop. Runtime remains authoritative
 * for capability discovery and execution admission; this value owns only the
 * immutable client-side behavior shared by the picker, composer and context
 * gauge.
 */
export class SelectableModel {
  readonly id: string;
  readonly provider: string;
  readonly label: string;
  readonly tokenLimits?: SelectableModelTokenLimits;
  readonly knowledgeCutoff?: string;
  readonly deprecated: boolean;
  readonly reasoning: boolean;
  readonly reasoningLevels: readonly string[];
  readonly reasoningDefaultLevel?: string;
  readonly inputModalities: readonly string[];
  readonly outputModalities: readonly string[];
  readonly toolUse: boolean;
  readonly structuredOutput: boolean;

  constructor(value: {
    id: string;
    provider: string;
    label: string;
    tokenLimits?: {
      contextWindow?: number;
      maxInputTokens?: number;
      maxOutputTokens?: number;
    };
    knowledgeCutoff?: string;
    deprecated?: boolean;
    reasoning?: boolean;
    reasoningLevels?: readonly string[];
    reasoningDefaultLevel?: string;
    inputModalities?: readonly string[];
    outputModalities?: readonly string[];
    toolUse?: boolean;
    structuredOutput?: boolean;
  }) {
    validateModelIdentity(value.id);
    validateProviderIdentity(value.provider);
    const reasoningLevels = value.reasoningLevels ?? [];
    for (const level of reasoningLevels) validateReasoningEffortIdentity(level);
    if (new Set(reasoningLevels).size !== reasoningLevels.length) {
      throw new Error("model reasoning levels are duplicated");
    }
    if (value.reasoningDefaultLevel !== undefined) {
      validateReasoningEffortIdentity(value.reasoningDefaultLevel);
      if (!reasoningLevels.includes(value.reasoningDefaultLevel)) {
        throw new Error("model default reasoning level is not offered");
      }
    }
    if (
      !(value.reasoning ?? false) &&
      (reasoningLevels.length !== 0 || value.reasoningDefaultLevel !== undefined)
    ) {
      throw new Error("non-reasoning model carries reasoning identities");
    }
    this.id = value.id;
    this.provider = value.provider;
    this.label = value.label;
    this.tokenLimits = value.tokenLimits
      ? new SelectableModelTokenLimits(value.tokenLimits)
      : undefined;
    this.knowledgeCutoff = value.knowledgeCutoff;
    this.deprecated = value.deprecated ?? false;
    this.reasoning = value.reasoning ?? false;
    this.reasoningLevels = Object.freeze([...reasoningLevels]);
    this.reasoningDefaultLevel = value.reasoningDefaultLevel;
    this.inputModalities = Object.freeze([...(value.inputModalities ?? [])]);
    this.outputModalities = Object.freeze([...(value.outputModalities ?? [])]);
    this.toolUse = value.toolUse ?? false;
    this.structuredOutput = value.structuredOutput ?? false;
    Object.freeze(this);
  }

  acceptsInput(modality: string): boolean {
    return this.inputModalities.includes(modality);
  }

  acceptsReasoningLevel(level: string): boolean {
    return this.reasoning && this.reasoningLevels.includes(level);
  }

  reasoningLevelOrDefault(level?: string | null): string | undefined {
    if (!this.reasoning) return undefined;
    if (level && this.acceptsReasoningLevel(level)) return level;
    if (this.reasoningDefaultLevel && this.acceptsReasoningLevel(this.reasoningDefaultLevel)) {
      return this.reasoningDefaultLevel;
    }
    return this.reasoningLevels[0];
  }
}
