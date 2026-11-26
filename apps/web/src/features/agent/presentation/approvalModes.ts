import { type Translate, type MessageKey } from "@/lib/i18n/index";
import { toolSourceLabel } from "@/features/agent/presentation/toolSource";
import {
  type ApprovalGate,
  type ApprovalMode,
  type ApprovalModePolicy,
} from "@/features/agent/domain/hitl";
import { type ApprovalRuleSummary } from "@/features/agent/application/approvalPolicyQueries";
export const APPROVAL_MODE_LABEL_KEY: Record<ApprovalMode, MessageKey> = {
  safe: "approvals.mode.safe",
  balanced: "approvals.mode.balanced",
  yolo: "approvals.mode.auto",
};

type ToolClass = keyof Omit<ApprovalModePolicy, "mode">;

const TOOL_CLASSES: ToolClass[] = ["write", "exec", "network"];

const TOOL_CLASS_KEY: Record<ToolClass, MessageKey> = {
  write: "approvals.toolClass.write",
  exec: "approvals.toolClass.exec",
  network: "approvals.toolClass.network",
};

const GATE_KEY: Record<ApprovalGate, MessageKey> = {
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

// A rule over all arguments and a built-in tool are the defaults, so only a narrower subject
// or an MCP source has anything to say. The label stays: an exact value may contain glob characters.
export function describeApprovalSubject(
  subject: ApprovalRuleSummary["subject"],
  t: Translate,
): { label: string; value: string } | undefined {
  return subject.type === "all"
    ? undefined
    : { label: t(`approvals.subject.${subject.type}`), value: subject.value };
}

export function describeApprovalRuleTool(tool: ApprovalRuleSummary["tool"]): string | undefined {
  return tool.type === "builtIn" ? undefined : toolSourceLabel(tool);
}
