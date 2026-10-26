import { useComposer } from "@/features/composer/public/composer";
import { type ComposerImage, type PastedText } from "@/features/composer/domain/draft";

export type { ComposerImage, PastedText } from "@/features/composer/domain/draft";

export function useComposerImages(): readonly ComposerImage[] {
  const composerAccess = useComposer();

  return composerAccess.useImages();
}

export function useComposerPastes(): readonly PastedText[] {
  const composerAccess = useComposer();

  return composerAccess.usePastes();
}

export function useAddComposerImageFiles(): (files: File[]) => void {
  const composerAccess = useComposer();

  return composerAccess.useAddImageFiles();
}

export function useRemoveComposerImage(): (id: string) => void {
  const composerAccess = useComposer();

  return composerAccess.useRemoveImage();
}

export function useAddComposerPaste(): (text: string) => void {
  const composerAccess = useComposer();

  return composerAccess.useAddPaste();
}

export function useRemoveComposerPaste(): (id: string) => void {
  const composerAccess = useComposer();

  return composerAccess.useRemovePaste();
}

export function useEditComposerPaste(): (id: string, text: string) => boolean {
  const composerAccess = useComposer();

  return composerAccess.useEditPaste();
}

export function useRestoreComposerPaste(): (id: string, text: string) => void {
  const composerAccess = useComposer();

  return composerAccess.useRestorePaste();
}
