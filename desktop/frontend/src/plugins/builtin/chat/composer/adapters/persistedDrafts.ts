// Untrusted input is validated HERE because here is where it enters, leaving the domain
// independent of localStorage and its validation library. Only the text is durable: staged
// images and pastes are heavy and meant to be sent immediately, so a reload drops them.

import { z } from "zod";
import type { ComposerDraftArchive } from "../domain/draftArchive";

const persistedDraftSchema = z.object({
  drafts: z.record(z.string(), z.object({ value: z.string() })),
});

export function persistedComposerDrafts(
  drafts: ComposerDraftArchive,
): Record<string, { value: string }> {
  return Object.fromEntries(
    Object.entries(drafts).map(([id, draft]) => [id, { value: draft.value }]),
  );
}

export function parsePersistedComposerDrafts(persisted: unknown): ComposerDraftArchive | null {
  const parsed = persistedDraftSchema.safeParse(persisted);
  if (!parsed.success) return null;
  const drafts: ComposerDraftArchive = {};
  for (const [id, draft] of Object.entries(parsed.data.drafts)) {
    drafts[id] = { value: draft.value, images: [], pastes: [] };
  }
  return drafts;
}
