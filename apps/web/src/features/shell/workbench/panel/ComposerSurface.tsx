import { useComposer } from "@/features/composer/public/composer";
import { memo } from "react";
import { Composer } from "@/features/composer/public/ui";
import { useSelectedModel } from "@/features/composer/public/selectedModel";
import {
  useAddComposerImageFiles,
  useAddComposerPaste,
  useComposerImageAttachments,
  useComposerPastes,
  useComposerReferences,
  useEditComposerPaste,
  useRemoveComposerImage,
  useRemoveComposerPaste,
  useRemoveComposerReference,
  useRestoreComposerPaste,
} from "@/features/composer/public/attachments";
export const ComposerSurface = memo(function ComposerSurface() {
  const key = useComposer().useDraftKey();
  const images = useComposerImageAttachments();
  const removeImage = useRemoveComposerImage();
  const addImageFiles = useAddComposerImageFiles();
  const pastes = useComposerPastes();
  const removePaste = useRemoveComposerPaste();
  const editPaste = useEditComposerPaste();
  const restorePaste = useRestoreComposerPaste();
  const addPaste = useAddComposerPaste();
  const references = useComposerReferences();
  const removeReference = useRemoveComposerReference();
  const acceptsImages = useSelectedModel()?.acceptsInput("image") ?? false;

  return (
    <Composer
      key={key}
      images={images}
      onRemoveImage={removeImage}
      onAddImages={addImageFiles}
      pastes={pastes}
      onRemovePaste={removePaste}
      onEditPaste={editPaste}
      onRestorePaste={restorePaste}
      onAddPaste={addPaste}
      references={references}
      onRemoveReference={removeReference}
      acceptsImages={acceptsImages}
    />
  );
});
