import type { SlashCommandSpec } from "@/plugins/sdk";

export interface SlashHintDefinition {
  cmd: string;
  descriptionKey: string;
}

export interface SlashHintContribution {
  cmd: string;
  spec: SlashCommandSpec;
}

export const DEFAULT_SLASH_HINTS: SlashHintDefinition[] = [
  { cmd: "/explain", descriptionKey: "slash.explain" },
  { cmd: "/test", descriptionKey: "slash.test" },
  { cmd: "/fix", descriptionKey: "slash.fix" },
  { cmd: "/diff", descriptionKey: "slash.diff" },
  { cmd: "/review", descriptionKey: "slash.review" },
  { cmd: "/commit", descriptionKey: "slash.commit" },
  { cmd: "/search", descriptionKey: "slash.search" },
  { cmd: "/plan", descriptionKey: "slash.plan" },
];

/**
 * Carries KEYS, not sentences: the suggestion list resolves them itself, so handing it
 * finished text pins every hint to whichever language was loaded at registration. It fails
 * silently — translating an already-translated string returns it unchanged.
 */
export function slashHintContributions(): SlashHintContribution[] {
  return DEFAULT_SLASH_HINTS.map((hint) => ({
    cmd: hint.cmd,
    spec: { description: hint.descriptionKey },
  }));
}
