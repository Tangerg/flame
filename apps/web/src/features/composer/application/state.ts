import {
  type ComposerDraftInput,
  type ComposerImage,
  type ComposerImageAttachment,
  type PastedText,
} from "@/features/composer/domain/draft";
export type ComposerModelPreference =
  | { kind: "session" }
  | { kind: "explicit"; provider: string; model: string; reasoningEffort?: string };

export interface ComposerSubmission {
  readonly value: string;
  readonly images: readonly ComposerImage[];
  readonly attachmentsReady: boolean;
  readonly pastes: readonly PastedText[];
  consume(historyText?: string): boolean;
}

export type ComposerDraftReplacement = (input: ComposerDraftInput) => boolean;

export type ComposerPersistenceState =
  | { status: "saved" }
  | { status: "loading" }
  | { status: "pending"; retrying?: boolean }
  | { status: "failed"; operation: "save"; cause: unknown }
  | { status: "failed"; operation: "read"; cause: unknown }
  | { status: "recovery-required"; cause: unknown };

export type ComposerLegacyDraftState =
  | { status: "none" }
  | { status: "available" | "blocked" | "importing" }
  | { status: "failed"; cause: unknown; canImport: boolean };

export interface ComposerStatePort {
  focus(selectionEnd?: number): void;
  captureFocus(): () => void;
  registerFocusTarget(
    element: HTMLTextAreaElement | null,
    synchronizeSelection?: () => void,
  ): () => void;
  registerFocusPresentation(present: (focus: () => void) => void): () => void;
  useText(): string;
  useSetText(): (value: string) => void;
  usePersistenceState(): ComposerPersistenceState;
  retryPersistence(): void;
  useLegacyDraftState(): ComposerLegacyDraftState;
  importLegacyDrafts(): void;
  getLegacyDraftBackup(): string;
  getRecoveryDrafts(): string;
  captureSubmission(): ComposerSubmission;
  replaceDraft(input: ComposerDraftInput): void;
  beginDraftReplacement(): ComposerDraftReplacement;
  appendText(text: string): void;
  useImageAttachments(): readonly ComposerImageAttachment[];
  useAttachmentsReady(): boolean;
  usePastes(): readonly PastedText[];
  useAddImageFiles(): (files: File[]) => void;
  useRemoveImage(): (id: string) => void;
  useAddPaste(): (text: string) => void;
  useRemovePaste(): (id: string) => void;
  useEditPaste(): (id: string, text: string) => boolean;
  useRestorePaste(): (id: string, text: string) => void;
  recallPreviousHistory(): boolean;
  recallNextHistory(): boolean;
  getModelPreference(): ComposerModelPreference;
  useModelPreference(): ComposerModelPreference;
  useSetModelPreference(): (preference: ComposerModelPreference) => void;
}
