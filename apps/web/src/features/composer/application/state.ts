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

export interface ComposerImageIntake {
  current(): boolean;
  addFiles(files: File[]): void;
}

export interface ComposerEditor {
  readonly element: HTMLElement;
  replace(text: string): void;
  append(text: string): void;
  focus(): void;
  revision(): object;
}

export interface ComposerStatePort {
  focus(): void;
  captureFocus(): () => void;
  registerFocusTarget(element: HTMLElement | null): () => void;
  registerFocusPresentation(present: (focus: () => void) => void): () => void;
  useDraftKey(): string;
  initialDocument(): { text: string; document: string | null };
  registerEditor(editor: ComposerEditor): () => void;
  checkpointDocument(text: string, document: string): void;
  useText(): string;
  useSetText(): (value: string) => void;
  usePersistenceState(): ComposerPersistenceState;
  retryPersistence(): void;
  getRecoveryDrafts(): string;
  captureSubmission(): ComposerSubmission;
  replaceDraft(input: ComposerDraftInput): void;
  beginDraftReplacement(): ComposerDraftReplacement;
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
