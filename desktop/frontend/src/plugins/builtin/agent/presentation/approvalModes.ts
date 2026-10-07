import type { Translate } from "@/lib/i18n";
import type { ApprovalGate, ApprovalMode, ApprovalModePolicy } from "../domain/hitl";

export const APPROVAL_MODE_LABEL_KEY: Record<ApprovalMode, string> = {
  safe: "approvals.mode.safe",
  balanced: "approvals.mode.balanced",
  yolo: "approvals.mode.auto",
};

type ToolClass = keyof Omit<ApprovalModePolicy, "mode">;

const TOOL_CLASSES: ToolClass[] = ["write", "exec", "network"];

const TOOL_CLASS_KEY: Record<ToolClass, string> = {
  write: "approvals.toolClass.write",
  exec: "approvals.toolClass.exec",
  network: "approvals.toolClass.network",
};

const GATE_KEY: Record<ApprovalGate, string> = {
  pass: "approvals.gate.pass",
  prompt: "approvals.gate.prompt",
  deny: "approvals.gate.deny",
};

export function describeApprovalMode(policy: ApprovalModePolicy, t: Translate): string {
  return TOOL_CLASSES.map((toolClass) =>
    t("approvals.gate.entry", {
      toolClass: t(TOOL_CLASS_KEY[toolClass]),
      gate: t(GATE_KEY[policy[toolClass]]),
    }),
  ).join(" · ");
}
