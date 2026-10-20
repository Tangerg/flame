import { isErrorType } from "@flame/runtime-client";
import { configureWorkspaceErrorClassifier } from "@/features/workspace/application/ports/workspaceErrorClassifier";
import { type WorkspaceErrorClassifier } from "@/features/workspace/application/ports/workspaceErrorClassifier";
const classifier: WorkspaceErrorClassifier = {
  isUnsupportedFile(error) {
    return isErrorType(error, "unsupported_mime");
  },
};

export function installWorkspaceErrorClassifier(): () => void {
  return configureWorkspaceErrorClassifier(classifier);
}
