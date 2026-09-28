import { z } from "zod";
import { Composer, type ComposerDraftText } from "../domain/composer";

const persistedDraftSchema = z.object({
  drafts: z.record(
    z.string(),
    z.object({
      value: z.string(),
      pastes: z
        .array(z.object({ id: z.string().min(1), text: z.string() }))
        .refine((pastes) => new Set(pastes.map((paste) => paste.id)).size === pastes.length),
    }),
  ),
});

export function persistedComposerDrafts(composer: Composer): Record<string, ComposerDraftText> {
  return Object.fromEntries(composer.durableDraftTexts());
}

export function parsePersistedComposer(persisted: unknown): Composer | null {
  const parsed = persistedDraftSchema.safeParse(persisted);
  if (!parsed.success) return null;
  return Composer.restoreDrafts(new Map(Object.entries(parsed.data.drafts)));
}
