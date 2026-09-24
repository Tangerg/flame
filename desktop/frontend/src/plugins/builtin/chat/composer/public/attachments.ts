import type { ComposerImage, PastedText } from "../domain/draft";
import { composerState } from "../application/ports/state";

export type { ComposerImage, PastedText } from "../domain/draft";

export function useComposerImages(): readonly ComposerImage[] {
  return composerState().useImages();
}

export function useComposerPastes(): readonly PastedText[] {
  return composerState().usePastes();
}

export function useAddComposerImageFiles(): (files: File[]) => void {
  return composerState().useAddImageFiles();
}

export function useRemoveComposerImage(): (id: string) => void {
  return composerState().useRemoveImage();
}

export function useAddComposerPaste(): (text: string) => void {
  return composerState().useAddPaste();
}

export function useRemoveComposerPaste(): (id: string) => void {
  return composerState().useRemovePaste();
}

export function useEditComposerPaste(): (id: string, text: string) => void {
  return composerState().useEditPaste();
}

export function useRestoreComposerPaste(): (id: string) => void {
  return composerState().useRestorePaste();
}
