import { isErrorType } from "@flame/runtime-contract/client";
import { configureWorkspaceErrorClassifier } from "../application/ports/workspaceErrorClassifier";
import type { WorkspaceErrorClassifier } from "../application/ports/workspaceErrorClassifier";

const classifier: WorkspaceErrorClassifier = {
  isVcsUnavailable(error) {
    return isErrorType(error, "vcs_unavailable");
  },
  isUnsupportedFile(error) {
    return isErrorType(error, "unsupported_mime");
  },
};

export function installWorkspaceErrorClassifier(): () => void {
  return configureWorkspaceErrorClassifier(classifier);
}
