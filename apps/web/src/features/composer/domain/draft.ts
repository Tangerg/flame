interface ComposerDraftImage {
  readonly mime: string;
  readonly data: string;
  readonly name?: string;
}

export interface ComposerDraftInput {
  text: string;
  images?: readonly ComposerDraftImage[];
}

export interface ComposerImage extends ComposerDraftImage {
  readonly id: string;
}

export type ComposerImageAttachment =
  | Readonly<ComposerImage & { status: "ready" }>
  | { readonly status: "pending" | "failed"; readonly id: string; readonly name: string };

export type ImageReadResult =
  { status: "ready"; image: Omit<ComposerImage, "id"> } | { status: "failed" };

export interface PastedText {
  readonly id: string;
  readonly text: string;
  readonly lines: number;
}

export function joinDraftParts(parts: readonly string[]): string {
  return parts.filter(Boolean).join("\n\n");
}
