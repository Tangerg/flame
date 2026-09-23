import { z } from "zod";
import { Composer } from "../domain/composer";

const persistedDraftSchema = z.object({
  drafts: z.record(z.string(), z.object({ value: z.string() })),
});

export function persistedComposerDrafts(composer: Composer): Record<string, { value: string }> {
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
