import type { ComposerImage, PastedText } from "@/plugins/builtin/chat/composer/public/attachments";
import type { AgentInput } from "@/plugins/builtin/agent/public/input";
import { useRecordComposerHistory } from "@/plugins/builtin/chat/composer/public/history";
import { TextArea } from "@/ui";
import { SUGGESTION_LISTBOX_ID, suggestionOptionId } from "../application/suggestions";
import { AgentComposerFooter, AgentComposerSurface } from "@/ui/agent";
import { FileMentionPopup } from "./FileMentionPopup";
import { SlashSuggestions } from "./SlashSuggestions";
import { composerStyles } from "./composerStyles";
import { useT } from "@/lib/i18n";
import { Slot } from "@/plugins/host/Slot";
import { ComposerAttachments } from "./ComposerAttachments";
import { ComposerDropCue, useComposerImageDrop } from "./ComposerImageDrop";
import { useComposerInputController } from "./useComposerInputController";
import { useRef } from "react";
import * as stylex from "@stylexjs/stylex";
import { type as typeStep } from "@/styles/tokens.stylex";

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
  onEditPaste: (id: string, text: string) => void;
  onRestorePaste: (id: string) => void;
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
  onEditPaste,
  onRestorePaste,
  onAddPaste,
  acceptsImages,
}: Props) {
  const t = useT();
  const surfaceRef = useRef<HTMLDivElement>(null);
  const recordHistory = useRecordComposerHistory();
  const {
    inputRef,
    mentions,
    slash,
    knownPaths,
    placeholder,
    steering,
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
  const dropping = useComposerImageDrop(handleDrop);
  const suggesting = mentions.open || slash.open;
  const highlighted =
    mentions.open && mentions.status === "ready" ? mentions.index : slash.open ? slash.index : null;
  return (
    <AgentComposerSurface
      ref={surfaceRef}
      data-slot="composer-root"
      data-dropping={dropping ? "" : undefined}
    >
      {dropping && <ComposerDropCue acceptsImages={acceptsImages} />}
      <FileMentionPopup mentions={mentions} anchor={surfaceRef} />
      <SlashSuggestions slash={slash} anchor={surfaceRef} />
      <div {...stylex.props(composerStyles.editorInset)}>
        <ComposerAttachments
          images={images}
          pastes={pastes}
          value={value}
          knownPaths={knownPaths}
          onChange={onChange}
          onRemoveImage={onRemoveImage}
          onRemovePaste={onRemovePaste}
          onEditPaste={onEditPaste}
          onRestorePaste={onRestorePaste}
        />
        <TextArea
          variant="bare"
          size="prose"
          ref={inputRef}
          aria-label={t("composer.input.label")}
          aria-controls={suggesting ? SUGGESTION_LISTBOX_ID : undefined}
          aria-activedescendant={highlighted === null ? undefined : suggestionOptionId(highlighted)}
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
          className={stylex.props(composerStyles.editor).className}
        />
        {steering && (
          <p
            data-slot="composer-steer-hint"
            aria-live="polite"
            {...stylex.props(composerStyles.steerHint, typeStep.uiXs)}
          >
            {t("composer.steer.hint")}
          </p>
        )}
      </div>
      <AgentComposerFooter>
        <Slot name="composer.toolbar.start" />
        <div {...stylex.props(composerStyles.toolbarSpacer)} />
        <Slot name="composer.toolbar.end" />
      </AgentComposerFooter>
    </AgentComposerSurface>
  );
}
