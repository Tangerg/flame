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

export function parseLegacyComposer(persisted: unknown): Composer | null {
  const parsed = persistedDraftSchema.safeParse(persisted);
  if (!parsed.success) return null;
  return Composer.restoreDrafts(new Map(Object.entries(parsed.data.drafts)));
}

export function decodeLegacyComposer(raw: string): Composer {
  const envelope = z.object({ state: z.unknown(), version: z.literal(0) }).parse(JSON.parse(raw));
  const composer = parseLegacyComposer(envelope.state);
  if (!composer) throw new Error("saved drafts have an invalid format");
  return composer;
}
