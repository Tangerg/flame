export class SelectableModelTokenLimits {
  readonly contextWindow?: number;
  readonly maxInputTokens?: number;
  readonly maxOutputTokens?: number;

  constructor(value: {
    contextWindow?: number;
    maxInputTokens?: number;
    maxOutputTokens?: number;
  }) {
    this.contextWindow = value.contextWindow;
    this.maxInputTokens = value.maxInputTokens;
    this.maxOutputTokens = value.maxOutputTokens;
    Object.freeze(this);
  }
}

export class SelectableModel {
  readonly id: string;
  readonly provider: string;
  readonly label: string;
  readonly tokenLimits?: SelectableModelTokenLimits;
  readonly knowledgeCutoff?: string;
  readonly deprecated: boolean;
  readonly default: boolean;
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
    default?: boolean;
    reasoning?: boolean;
    reasoningLevels?: readonly string[];
    reasoningDefaultLevel?: string;
    inputModalities?: readonly string[];
    outputModalities?: readonly string[];
    toolUse?: boolean;
    structuredOutput?: boolean;
  }) {
    const reasoningLevels = value.reasoningLevels ?? [];
    this.id = value.id;
    this.provider = value.provider;
    this.label = value.label;
    this.tokenLimits = value.tokenLimits
      ? new SelectableModelTokenLimits(value.tokenLimits)
      : undefined;
    this.knowledgeCutoff = value.knowledgeCutoff;
    this.deprecated = value.deprecated ?? false;
    this.default = value.default ?? false;
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
    if (level && this.acceptsReasoningLevel(level)) return level;
    return this.reasoningDefaultLevel;
  }
}
