import {
  type DraftContent,
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
export type RequestDraftReplacementConfirmation = (replace: () => boolean) => void;

export type ComposerPersistenceState =
  | { status: "saved" }
  | { status: "loading" }
  | { status: "pending"; retrying?: boolean }
  | { status: "failed"; operation: "save"; cause: unknown }
  | { status: "failed"; operation: "read"; cause: unknown }
  | { status: "recovery-required"; cause: unknown };

export interface ComposerImageIntake {
  current(): boolean;
  addFiles(files: File[]): void;
}

// The mounted editor owns the live document; the store keeps its checkpoint.
export interface ComposerEditor {
  readonly element: HTMLElement;
  restore(content: DraftContent): void;
  focus(): void;
  restoreFocusWithSelection(): boolean;
  revision(): object;
}

export interface ComposerStatePort {
  focus(): void;
  captureFocus(): () => void;
  captureEditorFocusReturn(target: HTMLElement): (() => boolean) | null;
  registerFocusTarget(element: HTMLElement | null): () => void;
  registerFocusPresentation(present: (focus: () => void) => void): () => void;
  useDraftKey(): string;
  initialContent(): DraftContent;
  registerEditor(editor: ComposerEditor): () => void;
  checkpoint(content: DraftContent): void;
  useText(): string;
  startFileReference(): string;
  startSlashCommand(): string;
  useDraftLoading(): boolean;
  usePersistenceState(): ComposerPersistenceState;
  retryPersistence(): void;
  getRecoveryDrafts(): string;
  captureSubmission(): ComposerSubmission;
  replaceDraft(
    input: ComposerDraftInput,
    requestConfirmation: RequestDraftReplacementConfirmation,
  ): void;
  beginDraftReplacement(
    requestConfirmation: RequestDraftReplacementConfirmation,
    begin: (replace: ComposerDraftReplacement) => boolean,
  ): void;
  appendText(text: string): void;
  captureAppendText(): (text: string) => boolean;
  useImageAttachments(): readonly ComposerImageAttachment[];
  useAttachmentsReady(): boolean;
  usePastes(): readonly PastedText[];
  useAddImageFiles(): (files: File[]) => void;
  useImageIntake(): ComposerImageIntake;
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
