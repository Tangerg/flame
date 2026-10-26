import { isErrorType } from "@flame/runtime-client";
import { type WorkspaceErrorClassifier } from "@/features/workspace/application/fileRead";
export const runtimeWorkspaceErrorClassifier: WorkspaceErrorClassifier = {
  isUnsupportedFile: (error) => isErrorType(error, "unsupported_mime"),
};
