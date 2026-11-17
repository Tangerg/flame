import { useComposer } from "@/features/composer/public/composer";
import { ComposerEditor } from "./ComposerEditor";
import { useNotifications } from "@/features/notifications/public/notifier";
import {
  type ComposerImageAttachment,
  type PastedText,
} from "@/features/composer/public/attachments";
import { type AgentInput } from "@/features/agent/public/input";
import { AgentComposerFooter, AgentComposerSurface } from "@/ui/agent/index";
import { SkeletonText } from "@/ui/index";
import { composerStyles } from "@/features/composer/ui/composerStyles";
import { useT } from "@/lib/i18n/index";
import { Slot } from "@/extensions/host/Slot";
import { ComposerAttachments } from "@/features/composer/ui/ComposerAttachments";
import { ComposerPersistenceNotice } from "@/features/composer/ui/ComposerPersistenceNotice";
import { ComposerFileDropCue, useComposerFileDrop } from "@/features/composer/ui/ComposerFileDrop";
import { useCallback, useRef } from "react";
import * as stylex from "@stylexjs/stylex";
interface Props {
  onSend: (input: AgentInput) => boolean;
  images: readonly ComposerImageAttachment[];
  onRemoveImage: (id: string) => void;
  onAddImages: (files: File[]) => void;
  pastes: readonly PastedText[];
  onRemovePaste: (id: string) => void;
  onEditPaste: (id: string, text: string) => boolean;
  onRestorePaste: (id: string, text: string) => void;
  onAddPaste: (text: string) => void;
  acceptsImages: boolean;
}

export function Composer({
  onSend,
  images,
  onRemoveImage,
  onAddImages,
  pastes,
  onRemovePaste,
  onEditPaste,
  onRestorePaste,
  onAddPaste,
  acceptsImages,
}: Props) {
  const t = useT();
  const surfaceRef = useRef<HTMLDivElement>(null);
  const composer = useComposer();
  const draftKey = composer.useDraftKey();
  const persistence = composer.usePersistenceState();
  const inputRef = useRef<HTMLDivElement>(null);
  const { messages } = useNotifications();
  const handleDrop = (
    files: File[],
    intake: import("@/features/composer/application/state").ComposerImageIntake,
  ) => {
    if (!acceptsImages) {
      messages.notify("error", t("composer.attachImage.unsupported"));
      return;
    }
    intake.addFiles(files);
  };
  const { ref: bindDrop, dragging: dropping } = useComposerFileDrop(acceptsImages, handleDrop);
  const bindSurface = useCallback(
    (node: HTMLDivElement | null) => {
      surfaceRef.current = node;
      bindDrop(node);
    },
    [bindDrop],
  );
  return (
    <AgentComposerSurface
      ref={bindSurface}
      data-slot="composer-root"
      data-dropping={dropping ? "" : undefined}
      className={stylex.props(composerStyles.surface).className}
    >
      {dropping && <ComposerFileDropCue acceptsImages={acceptsImages} />}
      <div {...stylex.props(composerStyles.editorInset)}>
        <ComposerPersistenceNotice />
        <ComposerAttachments
          images={images}
          pastes={pastes}
          onRemoveImage={onRemoveImage}
          onRemovePaste={onRemovePaste}
          onEditPaste={onEditPaste}
          onRestorePaste={onRestorePaste}
          composerInput={inputRef}
        />
        {persistence.status === "loading" ? (
          <SkeletonText label={t("common.loading")} />
        ) : (
          <ComposerEditor
            key={draftKey}
            inputRef={inputRef}
            onSend={onSend}
            onAddImages={onAddImages}
            onAddPaste={onAddPaste}
            acceptsImages={acceptsImages}
          />
        )}
      </div>
      <AgentComposerFooter>
        <Slot name="composer.toolbar.start" />
        <Slot
          name="composer.toolbar.end"
          wrapper
          className={stylex.props(composerStyles.toolbarActions).className}
        />
      </AgentComposerFooter>
    </AgentComposerSurface>
  );
}
