interface ComposerDraftImage {
  mime: string;
  data: string;
  name?: string;
}

export interface ComposerDraftInput {
  text: string;
  images?: readonly ComposerDraftImage[];
}

export interface ComposerImage extends ComposerDraftImage {
  id: string;
}

export interface PastedText {
  id: string;
  text: string;
  lines: number;
}

export function joinDraftParts(parts: readonly string[]): string {
  return parts.filter(Boolean).join("\n\n");
}
