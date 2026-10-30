export interface FileTransferPort {
  pickText(accept: string): Promise<string | null>;
}
