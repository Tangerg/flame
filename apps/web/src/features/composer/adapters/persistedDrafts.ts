import { z } from "zod";
import { Composer, type ComposerDraftText } from "@/features/composer/domain/composer";
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

export function encodePersistedComposer(composer: Composer): string {
  return JSON.stringify({ state: { drafts: persistedComposerDrafts(composer) }, version: 0 });
}

export function decodePersistedComposer(raw: string): Composer {
  const envelope = z.object({ state: z.unknown(), version: z.literal(0) }).parse(JSON.parse(raw));
  const composer = parsePersistedComposer(envelope.state);
  if (!composer) throw new Error("saved drafts have an invalid format");
  return composer;
}
