export interface WorkspaceErrorClassifier {
  isUnsupportedFile(error: unknown): boolean;
}
