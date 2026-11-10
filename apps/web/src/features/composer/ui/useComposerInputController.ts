import { useComposer } from "@/features/composer/public/composer";
import { useRuntime } from "@/features/connection/public/runtime";
import {
  type ChangeEvent,
  type ClipboardEvent,
  type CompositionEvent,
  type KeyboardEvent,
  type SyntheticEvent,
} from "react";
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { type AgentInput } from "@/features/agent/public/input";
import { imageFiles } from "@/features/composer/public/input";
import { resolvedWorkspaceCwd, useActiveSessionWorkspace } from "@/features/agent/public/session";
import {
  useFileMentions,
  useKnownWorkspacePaths,
} from "@/features/composer/application/fileMentions";
import { handleSuggestionKey } from "@/features/composer/application/suggestions";
import { useSlashSuggestions } from "@/features/composer/application/slashSuggestions";
import { useIsCurrentRootRunning } from "@/features/agent/public/run";
import { COMPOSER_KEY_BINDING } from "@/features/composer/public/extensionPoints";
import { lookupExtensionByKey } from "@/extensions/selectors/extensions";
import { notifyError } from "@/features/notifications/public/notifications";
import { submitComposer } from "@/features/composer/public/submit";
import {
  captureComposerFocus,
  registerComposerFocusTarget,
} from "@/features/composer/public/focus";
import { useT } from "@/lib/i18n/index";
import {
  composerKeyBindingKey,
  composerPasteIntent,
} from "@/features/composer/application/composerInputEvents";
import { useCompositionCommitGuard } from "@/features/composer/ui/useCompositionCommitGuard";
import { runtimeCommandsAvailable } from "@/features/connection/public/serviceStatus";
import { type ComposerSubmission } from "@/features/composer/application/state";

interface Args {
  value: string;
  onChange: (value: string) => void;
  captureSubmission: () => ComposerSubmission;
  onSend: (input: AgentInput) => boolean;
  onAddImages: (files: File[]) => void;
  onAddPaste: (text: string) => void;
  acceptsImages: boolean;
}

export function useComposerInputController({
  value,
  onChange,
  captureSubmission,
  onSend,
  onAddImages,
  onAddPaste,
  acceptsImages,
}: Args) {
  const composerAccess = useComposer();
  const runtimeAccess = useRuntime();

  const t = useT();
  const inputRef = useRef<HTMLTextAreaElement>(null);
  // The file-error Retry lives in the portaled popup, outside the textarea's
  // tab order, so the composer hands focus to it and takes it back.
  const retryRef = useRef<HTMLButtonElement>(null);
  const workspace = useActiveSessionWorkspace();
  const cwd = resolvedWorkspaceCwd(workspace);
  const [caret, setCaret] = useState(0);
  const composition = useCompositionCommitGuard();
  const synchronizeCaret = useCallback(() => {
    const input = inputRef.current;
    if (input && input.ownerDocument.activeElement === input && !composition.isComposing()) {
      setCaret(input.selectionStart);
    }
  }, [composition]);
  const returnFocusToInput = useCallback(() => {
    inputRef.current?.focus();
    synchronizeCaret();
  }, [synchronizeCaret]);
  useEffect(
    () => registerComposerFocusTarget(composerAccess, inputRef.current, synchronizeCaret),
    [composerAccess, synchronizeCaret],
  );
  const acceptedSelection = useRef<(() => void) | null>(null);
  useLayoutEffect(() => {
    const settle = acceptedSelection.current;
    acceptedSelection.current = null;
    if (settle) settle();
    else synchronizeCaret();
  }, [value, synchronizeCaret]);
  const applyMention = useCallback(
    (text: string, next: number) => {
      const input = inputRef.current;
      const restoreFocus = captureComposerFocus(composerAccess);
      // Settle the caret with the accepted text before a following keystroke can arrive.
      acceptedSelection.current = () => {
        if (!input || inputRef.current !== input || input.value !== text) return;
        restoreFocus();
        if (input.ownerDocument.activeElement === input) {
          input.setSelectionRange(next, next);
          synchronizeCaret();
        }
      };
      onChange(text);
    },
    [onChange, composerAccess, synchronizeCaret],
  );
  const mentions = useFileMentions({ value, caret, cwd, apply: applyMention });
  const slash = useSlashSuggestions({ value, caret, apply: applyMention });
  const knownPaths = useKnownWorkspacePaths(cwd, value);
  const running = useIsCurrentRootRunning();
  const placeholder = running ? t("composer.placeholder.steer") : t("composer.placeholder");
  const submit = useCallback(
    () =>
      submitComposer({
        draft: captureSubmission(),
        restoreFocus: captureComposerFocus(composerAccess),
        sendInput: onSend,
        canSend: runtimeCommandsAvailable.bind(null, runtimeAccess),
      }),
    [captureSubmission, onSend, composerAccess, runtimeAccess],
  );

  const handleChange = (event: ChangeEvent<HTMLTextAreaElement>): void => {
    const target = event.target;
    const nativeComposing = (event.nativeEvent as { isComposing?: boolean }).isComposing === true;
    composition.noteChange(nativeComposing);
    onChange(target.value);
    if (composition.isComposing() || nativeComposing) return;
    setCaret(target.selectionStart ?? target.value.length);
  };

  const handleSelect = (event: SyntheticEvent<HTMLTextAreaElement>): void => {
    if (composition.isComposing()) return;
    setCaret(event.currentTarget.selectionStart ?? 0);
  };

  const handleCompositionStart = (): void => {
    composition.start();
  };

  const handleCompositionEnd = (event: CompositionEvent<HTMLTextAreaElement>): void => {
    composition.end();
    const target = event.currentTarget;
    onChange(target.value);
    setCaret(target.selectionStart ?? target.value.length);
  };

  const handlePaste = (event: ClipboardEvent<HTMLTextAreaElement>): void => {
    composition.clear();
    const files = imageFiles(event.clipboardData?.files);
    const text = event.clipboardData?.getData("text") ?? "";
    const intent = composerPasteIntent(files, text, acceptsImages);
    if (files.length > 0 && !acceptsImages) notifyError(t("composer.attachImage.unsupported"));
    switch (intent.kind) {
      case "images":
        event.preventDefault();
        onAddImages(intent.files);
        break;
      case "unsupported-images":
        event.preventDefault();
        break;
      case "large-text":
        event.preventDefault();
        onAddPaste(intent.text);
        break;
    }
  };

  const handleDrop = (files: File[]): void => {
    composition.clear();
    if (files.length === 0) return;
    if (!acceptsImages) {
      notifyError(t("composer.attachImage.unsupported"));
      return;
    }
    onAddImages(files);
  };

  const clearCompositionCommit = (): void => {
    composition.clear();
  };

  const handleKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>): void => {
    const compositionIntent = composition.keyIntent(event.nativeEvent);
    if (compositionIntent !== null) {
      if (compositionIntent === "committed-enter") event.preventDefault();
      return;
    }
    const retry = retryRef.current;
    if (
      retry &&
      mentions.open &&
      mentions.status === "error" &&
      event.key === "Tab" &&
      !event.shiftKey &&
      !event.altKey &&
      !event.ctrlKey &&
      !event.metaKey
    ) {
      event.preventDefault();
      retry.focus();
      return;
    }
    if (handleSuggestionKey(mentions, event) || handleSuggestionKey(slash, event)) {
      event.preventDefault();
      return;
    }
    const binding = lookupExtensionByKey(
      COMPOSER_KEY_BINDING,
      composerKeyBindingKey(event.nativeEvent),
    );
    if (!binding) return;
    const handled = binding.handler({
      value,
      onChange,
      submit,
      event: event.nativeEvent,
    });
    if (handled) event.preventDefault();
  };

  return {
    inputRef,
    retryRef,
    returnFocusToInput,
    mentions,
    slash,
    knownPaths,
    placeholder,
    handleChange,
    clearCompositionCommit,
    handleCompositionStart,
    handleCompositionEnd,
    handleDrop,
    handleKeyDown,
    handleKeyUp: clearCompositionCommit,
    handlePaste,
    handleSelect,
  };
}
