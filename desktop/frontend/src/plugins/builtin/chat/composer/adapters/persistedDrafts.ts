// Untrusted input is validated HERE because here is where it enters, leaving the domain
// independent of localStorage and its validation library.

import { z } from "zod";
import { Composer } from "../domain/composer";

const persistedDraftSchema = z.object({
  drafts: z.record(z.string(), z.object({ value: z.string() })),
});

export function persistedComposerDrafts(composer: Composer): Record<string, { value: string }> {
  // `fromEntries`, not key-by-key assignment: a session id of `__proto__` assigns nothing
  // to an object literal, so the draft would vanish without a word.
  return Object.fromEntries(
    [...composer.durableDraftTexts()].map(([id, value]) => [id, { value }]),
  );
}

export function parsePersistedComposer(persisted: unknown): Composer | null {
  const parsed = persistedDraftSchema.safeParse(persisted);
  if (!parsed.success) return null;
  return Composer.restoreDrafts(
    new Map(Object.entries(parsed.data.drafts).map(([id, draft]) => [id, draft.value])),
  );
}
