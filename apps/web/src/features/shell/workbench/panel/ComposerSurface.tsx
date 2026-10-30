import { useComposer } from "@/features/composer/public/composer";
import { memo } from "react";
import { type AgentInput } from "@/features/agent/public/input";
import { Composer } from "@/features/composer/public/ui";
import { useSelectedModel } from "@/features/composer/public/selectedModel";
import {
  useAddComposerImageFiles,
  useAddComposerPaste,
  useComposerImageAttachments,
  useComposerPastes,
  useEditComposerPaste,
  useRemoveComposerImage,
  useRemoveComposerPaste,
  useRestoreComposerPaste,
} from "@/features/composer/public/attachments";
import {
  captureComposerSubmission,
  useComposerText,
  useSetComposerText,
} from "@/features/composer/public/draft";
export const ComposerSurface = memo(function ComposerSurface({
  onSend,
}: {
  onSend: (input: AgentInput) => boolean;
}) {
  const composerAccess = useComposer();

  const value = useComposerText();
  const setValue = useSetComposerText();
  const images = useComposerImageAttachments();
  const removeImage = useRemoveComposerImage();
  const addImageFiles = useAddComposerImageFiles();
  const pastes = useComposerPastes();
  const removePaste = useRemoveComposerPaste();
  const editPaste = useEditComposerPaste();
  const restorePaste = useRestoreComposerPaste();
  const addPaste = useAddComposerPaste();
  const acceptsImages = useSelectedModel()?.acceptsInput("image") ?? false;

  return (
    <Composer
      value={value}
      onChange={setValue}
      captureSubmission={captureComposerSubmission.bind(null, composerAccess)}
      onSend={onSend}
      images={images}
      onRemoveImage={removeImage}
      onAddImages={addImageFiles}
      pastes={pastes}
      onRemovePaste={removePaste}
      onEditPaste={editPaste}
      onRestorePaste={restorePaste}
      onAddPaste={addPaste}
      acceptsImages={acceptsImages}
    />
  );
});
