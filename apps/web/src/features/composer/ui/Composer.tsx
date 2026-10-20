import { type ComposerImage, type PastedText } from "@/features/composer/public/attachments";
import { type AgentInput } from "@/features/agent/public/input";
import { TextArea } from "@flame/ui";
import {
  SUGGESTION_LISTBOX_ID,
  suggestionOptionId,
} from "@/features/composer/application/suggestions";
import { AgentComposerFooter, AgentComposerSurface } from "@/ui/agent/index";
import { FileMentionPopup } from "@/features/composer/ui/FileMentionPopup";
import { SlashSuggestions } from "@/features/composer/ui/SlashSuggestions";
import { composerStyles } from "@/features/composer/ui/composerStyles";
import { useT } from "@/lib/i18n/index";
import { type ComposerSubmission } from "@/features/composer/application/ports/state";
import { Slot } from "@/extensions/host/Slot";
import { ComposerAttachments } from "@/features/composer/ui/ComposerAttachments";
import { ComposerDropCue, useComposerImageDrop } from "@/features/composer/ui/ComposerImageDrop";
import { useComposerInputController } from "@/features/composer/ui/useComposerInputController";
import { useRef } from "react";
import * as stylex from "@stylexjs/stylex";
import { type as typeStep } from "@flame/ui/tokens.stylex";
interface Props {
  onSend: (input: AgentInput) => boolean;
  value: string;
  onChange: (v: string) => void;
  captureSubmission: () => ComposerSubmission;
  images: readonly ComposerImage[];
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
  value,
  onChange,
  captureSubmission,
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
    captureSubmission,
    onSend,
    images,
    pastes,
    onAddImages,
    onAddPaste,
    acceptsImages,
  });
  const dropping = useComposerImageDrop(handleDrop);
  const listing = slash.open || (mentions.open && mentions.status === "ready");
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
          composerInput={inputRef}
        />
        <TextArea
          variant="bare"
          data-chrome-focus=""
          size="prose"
          ref={inputRef}
          aria-label={t("composer.input.label")}
          aria-controls={listing ? SUGGESTION_LISTBOX_ID : undefined}
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
