import {
  type ComposerImageAttachment,
  type PastedText,
} from "@/features/composer/public/attachments";
import { type AgentInput } from "@/features/agent/public/input";
import { TextArea } from "@flame/ui";
import { AgentComposerFooter, AgentComposerSurface, useViewportFocus } from "@/ui/agent/index";
import { useContentVisible } from "@/ui/index";
import {
  FileMentionOptions,
  FileMentionStatus,
} from "@/features/composer/ui/FileMentionSuggestions";
import { SlashSuggestions } from "@/features/composer/ui/SlashSuggestions";
import { SuggestionPopup } from "@/features/composer/ui/SuggestionPopup";
import { composerStyles } from "@/features/composer/ui/composerStyles";
import { useT } from "@/lib/i18n/index";
import { type ComposerSubmission } from "@/features/composer/application/state";
import { Slot } from "@/extensions/host/Slot";
import { ComposerAttachments } from "@/features/composer/ui/ComposerAttachments";
import { ComposerPersistenceNotice } from "@/features/composer/ui/ComposerPersistenceNotice";
import { ComposerFileDropCue, useComposerFileDrop } from "@/features/composer/ui/ComposerFileDrop";
import { useComposerInputController } from "@/features/composer/ui/useComposerInputController";
import { useRef } from "react";
import * as stylex from "@stylexjs/stylex";
interface Props {
  onSend: (input: AgentInput) => boolean;
  value: string;
  onChange: (v: string) => void;
  captureSubmission: () => ComposerSubmission;
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
  const editorRef = useRef<HTMLDivElement>(null);
  const visible = useContentVisible();
  useViewportFocus(editorRef, visible);
  const {
    inputRef,
    bindInput,
    retryRef,
    returnFocusToInput,
    mentions,
    slash,
    suggestions,
    navigation,
    knownPaths,
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
    captureSubmission,
    onSend,
    onAddImages,
    onAddPaste,
    acceptsImages,
  });
  const dropping = useComposerFileDrop(surfaceRef, acceptsImages, handleDrop);
  return (
    <AgentComposerSurface
      ref={surfaceRef}
      data-slot="composer-root"
      data-dropping={dropping ? "" : undefined}
      className={stylex.props(composerStyles.surface).className}
    >
      {dropping && <ComposerFileDropCue acceptsImages={acceptsImages} />}
      <SuggestionPopup
        open={suggestions.open}
        onDismiss={suggestions.dismiss}
        navigation={navigation}
        heading={t(mentions.open ? "composer.mention.heading" : "composer.slash.heading")}
        anchor={surfaceRef}
        status={
          mentions.open && mentions.status !== "ready" ? (
            <FileMentionStatus
              mentions={mentions}
              retryRef={retryRef}
              onReturnFocus={returnFocusToInput}
            />
          ) : undefined
        }
      >
        {mentions.open ? (
          <FileMentionOptions mentions={mentions} navigation={navigation} />
        ) : (
          <SlashSuggestions slash={slash} navigation={navigation} />
        )}
      </SuggestionPopup>
      <div ref={editorRef} {...stylex.props(composerStyles.editorInset)}>
        <ComposerPersistenceNotice />
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
          {...navigation.referenceProps}
          ref={bindInput}
          aria-label={t("composer.input.label")}
          placeholder={placeholder}
          value={value}
          onChange={handleChange}
          onSelect={handleSelect}
          onBlur={(event) => {
            navigation.referenceProps.onBlur?.(event);
            clearCompositionCommit();
          }}
          onFocus={(event) => {
            navigation.referenceProps.onFocus?.(event);
            clearCompositionCommit();
          }}
          onPointerUp={(event) => {
            navigation.referenceProps.onPointerUp?.(event);
            clearCompositionCommit();
          }}
          onCompositionStart={handleCompositionStart}
          onCompositionEnd={handleCompositionEnd}
          onPaste={handlePaste}
          onKeyDown={handleKeyDown}
          onKeyUp={handleKeyUp}
          rows={1}
          autosize
          className={stylex.props(composerStyles.editor).className}
        />
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
