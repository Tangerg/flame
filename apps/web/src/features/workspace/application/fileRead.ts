export interface WorkspaceErrorClassifier {
  isUnsupportedFile(error: unknown): boolean;
  isInvalidReadRequest(error: unknown): boolean;
}
