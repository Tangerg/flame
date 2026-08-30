import {
  MAXIMUM_MODEL_IDENTITY_CHARACTERS,
  MAXIMUM_PROVIDER_IDENTITY_CHARACTERS,
  MAXIMUM_REASONING_EFFORT_IDENTITY_CHARACTERS,
} from "@flame/runtime-contract/wire";

const NON_IDENTITY_CHARACTER = /[\p{C}\p{Z}]/u;
type IdentityKind = "provider" | "model" | "reasoningEffort";

function validateIdentity(kind: IdentityKind, value: string, maximumCharacters: number): void {
  if (value.length === 0) throw new Error(`${kind}_identity_empty`);
  if (Array.from(value).length > maximumCharacters) {
    throw new Error(`${kind}_identity_exceeds_${maximumCharacters}_characters`);
  }
  if (NON_IDENTITY_CHARACTER.test(value)) {
    throw new Error(`${kind}_identity_not_canonical`);
  }
}

export function validateProviderIdentity(value: string): void {
  validateIdentity("provider", value, MAXIMUM_PROVIDER_IDENTITY_CHARACTERS);
}

export function validateModelIdentity(value: string): void {
  validateIdentity("model", value, MAXIMUM_MODEL_IDENTITY_CHARACTERS);
}

export function validateReasoningEffortIdentity(value: string): void {
  validateIdentity("reasoningEffort", value, MAXIMUM_REASONING_EFFORT_IDENTITY_CHARACTERS);
}
