import type { ApprovalMode } from "../domain/hitl";

export interface ApprovalModeOption {
  value: ApprovalMode;
  labelKey: string;
  descKey: string;
}

export const APPROVAL_MODE_OPTION: Record<ApprovalMode, ApprovalModeOption> = {
  safe: { value: "safe", labelKey: "approvals.mode.safe", descKey: "approvals.mode.safe.desc" },
  balanced: {
    value: "balanced",
    labelKey: "approvals.mode.balanced",
    descKey: "approvals.mode.balanced.desc",
  },
  yolo: { value: "yolo", labelKey: "approvals.mode.auto", descKey: "approvals.mode.auto.desc" },
};

export const APPROVAL_MODES: ApprovalModeOption[] = [
  APPROVAL_MODE_OPTION.safe,
  APPROVAL_MODE_OPTION.balanced,
  APPROVAL_MODE_OPTION.yolo,
];
