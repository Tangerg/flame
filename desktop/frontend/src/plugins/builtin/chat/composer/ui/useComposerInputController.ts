import type {
  ChangeEvent,
  ClipboardEvent,
  CompositionEvent,
  KeyboardEvent,
  SyntheticEvent,
} from "react";
import { useCallback, useEffect, useRef, useState } from "react";
import type { ComposerImage, PastedText } from "@/plugins/builtin/chat/composer/public/attachments";
import type { AgentInput } from "@/plugins/builtin/agent/public/input";
import { imageFiles } from "@/plugins/builtin/chat/composer/public/input";
import { useActiveSessionWorkspace } from "@/plugins/builtin/agent/public/session";
import { useFileMentions } from "@/plugins/builtin/chat/composer/public/fileMentions";
import { useIsCurrentRootRunning } from "@/plugins/builtin/agent/public/run";
import { COMPOSER_KEY_BINDING, lookupExtensionByKey } from "@/plugins/sdk";
import { submitComposer } from "@/plugins/builtin/chat/composer/public/submit";
import { setComposerFocusTarget } from "../application/focus";
import { useT } from "@/lib/i18n";
import {
  composerCompositionKeyIntent,
  composerKeyBindingKey,
  composerPasteIntent,
} from "../application/composerInputEvents";
import { runtimeCommandsAvailable } from "@/plugins/builtin/runtime/public/serviceStatus";

/**
 * How long after a composition commits an ordinary Enter can still be the IME's own.
 *
 * The flag this bounds exists for one shape: some Chinese IMEs commit with `compositionend`
 * and then emit a completely ordinary Enter from the SAME physical keypress, which would
 * otherwise send a half-written message. That Enter arrives in the same input burst.
 *
 * Unbounded, the flag also swallows the next Enter after a commit that involved no key at all
 * — picking a candidate with the mouse. Measured: `compositionend` with no key events, then
 * Enter, and nothing was sent; the reader has to press Enter twice. Nothing distinguishes the
 * two Enters except when they arrive, so the window is the discriminator: far above the
 * browser's dispatch gap, far below moving a hand from the mouse to the keyboard.
 *
 * It is deliberately the loose side of that gap. Too tight and a commit sends an unfinished
 * message; too loose and one Enter is ignored — only one of those loses text.
 */
const COMPOSITION_COMMIT_GRACE_MS = 100;

interface Args {
  value: string;
  onChange: (value: string) => void;
  onClear: () => void;
  onSend: (input: AgentInput) => boolean;
  images: readonly ComposerImage[];
  pastes: readonly PastedText[];
  recordHistory: (text: string) => void;
  onAddImages: (files: File[]) => void;
  onAddPaste: (text: string) => void;
  acceptsImages: boolean;
}

export function useComposerInputController({
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
}: Args) {
  const t = useT();
  const inputRef = useRef<HTMLTextAreaElement>(null);
  useEffect(() => {
    setComposerFocusTarget(inputRef.current);
    return () => setComposerFocusTarget(null);
  }, []);
  const workspace = useActiveSessionWorkspace();
  const cwd = workspace.status === "ready" ? workspace.cwd : undefined;
  const [caret, setCaret] = useState(0);
  const composingRef = useRef(false);
  // WHEN the composition committed, not merely that it did — see the grace window above.
  const compositionCommittedAtRef = useRef<number | null>(null);
  const commitIsFresh = (): boolean => {
    const at = compositionCommittedAtRef.current;
    return at !== null && performance.now() - at <= COMPOSITION_COMMIT_GRACE_MS;
  };
  const applyMention = useCallback(
    (text: string, next: number) => {
      onChange(text);
      requestAnimationFrame(() => {
        const textarea = inputRef.current;
        if (textarea) {
          textarea.focus();
          textarea.setSelectionRange(next, next);
        }
        setCaret(next);
      });
    },
    [onChange],
  );
  const mentions = useFileMentions({ value, caret, cwd, apply: applyMention });
  const running = useIsCurrentRootRunning();
  const placeholder = running ? t("composer.placeholder.steer") : t("composer.placeholder");
  const submit = useCallback(
    () =>
      submitComposer({
        value,
        clear: onClear,
        sendInput: onSend,
        images,
        pastes,
        recordHistory,
        canSend: runtimeCommandsAvailable,
      }),
    [images, onClear, onSend, pastes, recordHistory, value],
  );

  const handleChange = (event: ChangeEvent<HTMLTextAreaElement>): void => {
    const target = event.target;
    const nativeComposing = (event.nativeEvent as { isComposing?: boolean }).isComposing === true;
    if (composingRef.current && !nativeComposing) {
      composingRef.current = false;
      compositionCommittedAtRef.current = performance.now();
    }
    onChange(target.value);
    if (composingRef.current || nativeComposing) return;
    setCaret(target.selectionStart ?? target.value.length);
  };

  const handleSelect = (event: SyntheticEvent<HTMLTextAreaElement>): void => {
    if (composingRef.current) return;
    setCaret(event.currentTarget.selectionStart ?? 0);
  };

  const handleCompositionStart = (): void => {
    composingRef.current = true;
    compositionCommittedAtRef.current = null;
  };

  const handleCompositionEnd = (event: CompositionEvent<HTMLTextAreaElement>): void => {
    composingRef.current = false;
    compositionCommittedAtRef.current = performance.now();
    const target = event.currentTarget;
    onChange(target.value);
    setCaret(target.selectionStart ?? target.value.length);
  };

  const handlePaste = (event: ClipboardEvent<HTMLTextAreaElement>): void => {
    compositionCommittedAtRef.current = null;
    const files = imageFiles(event.clipboardData?.files);
    const text = files.length > 0 ? "" : (event.clipboardData?.getData("text") ?? "");
    const intent = composerPasteIntent(files, text);
    switch (intent.kind) {
      case "images":
        event.preventDefault();
        if (acceptsImages) onAddImages(intent.files);
        break;
      case "large-text":
        event.preventDefault();
        onAddPaste(intent.text);
        break;
    }
  };

  const handleDrop = (files: File[]): void => {
    compositionCommittedAtRef.current = null;
    if (files.length === 0 || !acceptsImages) return;
    onAddImages(files);
  };

  const clearCompositionCommit = (): void => {
    compositionCommittedAtRef.current = null;
  };

  const handleKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>): void => {
    const compositionIntent = composerCompositionKeyIntent(
      event.nativeEvent,
      composingRef.current,
      commitIsFresh(),
    );
    compositionCommittedAtRef.current = null;
    if (compositionIntent !== null) {
      if (compositionIntent === "committed-enter") event.preventDefault();
      return;
    }
    if (mentions.handleKeyDown(event)) {
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
    mentions,
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
