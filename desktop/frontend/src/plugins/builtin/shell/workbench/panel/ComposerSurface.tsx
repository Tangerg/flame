import { memo } from "react";
import type { AgentInput } from "@/plugins/builtin/agent/public/input";
import { Composer } from "@/plugins/builtin/chat/composer/public/ui";
import { useSelectedModel } from "@/plugins/builtin/chat/composer/public/selectedModel";
import {
  useAddComposerImageFiles,
  useAddComposerPaste,
  useComposerImages,
  useComposerPastes,
  useEditComposerPaste,
  useRemoveComposerImage,
  useRemoveComposerPaste,
  useRestoreComposerPaste,
} from "@/plugins/builtin/chat/composer/public/attachments";
import {
  captureComposerSubmission,
  useComposerText,
  useSetComposerText,
} from "@/plugins/builtin/chat/composer/public/draft";

export const ComposerSurface = memo(function ComposerSurface({
  onSend,
}: {
  onSend: (input: AgentInput) => boolean;
}) {
  const value = useComposerText();
  const setValue = useSetComposerText();
  const images = useComposerImages();
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
      captureSubmission={captureComposerSubmission}
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
