import { type Translate } from "@/lib/i18n/index";
import { toolSourceLabel } from "@/lib/toolSource";
import {
  type ApprovalGate,
  type ApprovalMode,
  type ApprovalModePolicy,
} from "@/features/agent/domain/hitl";
import { type ApprovalRuleSummary } from "@/features/agent/application/approvalPolicyQueries";
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

// The label is prose and the value is the matched argument, so callers set them in different faces.
export function describeApprovalSubject(
  subject: ApprovalRuleSummary["subject"],
  t: Translate,
): { label: string; value?: string } {
  const label = t(`approvals.subject.${subject.type}`);
  return subject.type === "all" ? { label } : { label, value: subject.value };
}

export function describeApprovalRuleTool(tool: ApprovalRuleSummary["tool"], t: Translate): string {
  return tool.type === "builtIn" ? t("approvals.builtIn") : toolSourceLabel(tool);
}
