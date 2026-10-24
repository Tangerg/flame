export interface FileTransferPort {
  // Awaits the host's file handoff, including native selection and writing.
  saveText(filename: string, content: string, mime: string): Promise<void>;
  pickText(accept: string): Promise<string | null>;
}
