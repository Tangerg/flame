/**
 * A port because the use case — "export this conversation as markdown" — is ours while the
 * browser owns the mechanism.
 */
export interface FileTransferPort {
  download(filename: string, content: string, mime: string): void;
  /** Resolves the chosen file's text, or null when the picker is cancelled. */
  pickText(accept: string): Promise<string | null>;
}
