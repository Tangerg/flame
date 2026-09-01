import type { ComposerImage, PastedText } from "@/plugins/builtin/chat/composer/public/attachments";
import type { AgentInput } from "@/plugins/builtin/agent/public/input";
import { useRecordComposerHistory } from "@/plugins/builtin/chat/composer/public/history";
import { TextArea } from "@/ui";
import {
  MENTION_LISTBOX_ID,
  mentionOptionId,
} from "@/plugins/builtin/chat/composer/application/fileMentions";
import { AgentComposerSurface } from "@/ui/agent";
import { FileMentionPopup } from "./FileMentionPopup";
import { useT } from "@/lib/i18n";
import { Slot } from "@/plugins/host/Slot";
import { ComposerAttachments } from "./ComposerAttachments";
import { ComposerImageDrop } from "./ComposerImageDrop";
import { useComposerInputController } from "./useComposerInputController";

interface Props {
  onSend: (input: AgentInput) => boolean;
  value: string;
  onChange: (v: string) => void;
  onClear: () => void;
  images: readonly ComposerImage[];
  onRemoveImage: (id: string) => void;
  onAddImages: (files: File[]) => void;
  pastes: readonly PastedText[];
  onRemovePaste: (id: string) => void;
  onAddPaste: (text: string) => void;
  acceptsImages: boolean;
}

export function Composer({
  onSend,
  value,
  onChange,
  onClear,
  images,
  onRemoveImage,
  onAddImages,
  pastes,
  onRemovePaste,
  onAddPaste,
  acceptsImages,
}: Props) {
  const t = useT();
  const recordHistory = useRecordComposerHistory();
  const {
    inputRef,
    mentions,
    placeholder,
    handleChange,
    clearCompositionCommit,
    handleCompositionStart,
    handleCompositionEnd,
    handleDrop,
    handleKeyDown,
    handleKeyUp,
    handlePaste,
    handleSelect,
  } = useComposerInputController({
    value,
    onChange,
    onClear,
    onSend,
    images,
    pastes,
    recordHistory,
    onAddImages,
    onAddPaste,
    acceptsImages,
  });
  return (
    <AgentComposerSurface className="relative" data-slot="composer-root">
      <ComposerImageDrop enabled={acceptsImages} onDropImages={handleDrop} />
      {mentions.active && (
        <FileMentionPopup
          items={mentions.items}
          index={mentions.index}
          onPick={mentions.accept}
          onHover={mentions.setIndex}
        />
      )}
      <div className="pt-[var(--density-composer-editor-top)] pr-[var(--density-composer-editor-end)] pb-[var(--density-composer-editor-bottom)] pl-[var(--density-composer-editor-start)]">
        <ComposerAttachments
          images={images}
          pastes={pastes}
          value={value}
          onChange={onChange}
          onRemoveImage={onRemoveImage}
          onRemovePaste={onRemovePaste}
        />
        <TextArea
          variant="bare"
          size="prose"
          font="sans"
          ref={inputRef}
          aria-label={t("composer.input.label")}
          aria-controls={mentions.active ? MENTION_LISTBOX_ID : undefined}
          aria-activedescendant={mentions.active ? mentionOptionId(mentions.index) : undefined}
          placeholder={placeholder}
          value={value}
          onChange={handleChange}
          onSelect={handleSelect}
          onBlur={clearCompositionCommit}
          onFocus={clearCompositionCommit}
          onCompositionStart={handleCompositionStart}
          onCompositionEnd={handleCompositionEnd}
          onPaste={handlePaste}
          onKeyDown={handleKeyDown}
          onKeyUp={handleKeyUp}
          onPointerUp={clearCompositionCommit}
          rows={1}
          autosize
          className="max-h-[6lh] min-h-[1.5lh] p-0 placeholder:tracking-normal"
        />
      </div>
      <div
        data-slot="composer-footer"
        className="agent-composer-footer flex flex-nowrap items-center gap-1.5 pr-[var(--density-composer-footer-end)] pb-[var(--density-composer-footer)] pl-[var(--density-composer-footer)]"
      >
        <Slot name="composer.toolbar.start" />
        <div className="flex-1 min-w-2" />
        <Slot name="composer.toolbar.end" />
      </div>
    </AgentComposerSurface>
  );
}
