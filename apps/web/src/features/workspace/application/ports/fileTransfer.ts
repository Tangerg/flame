export interface FileTransferPort {
  download(filename: string, content: string, mime: string): void;
  pickText(accept: string): Promise<string | null>;
}
