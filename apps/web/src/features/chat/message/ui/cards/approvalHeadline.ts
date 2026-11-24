import { type Translate } from "@/lib/i18n/index";
import { type ToolCategory } from "@/features/agent/public/sessionView";
export function approvalHeadline(
  t: Translate,
  toolName: string | undefined,
  category: ToolCategory | undefined,
): string {
  if (!toolName) return t("approval.fallbackText");
  switch (category) {
    case "shell":
      return t("approval.what.command");
    case "edit":
      return t("approval.what.fileEdit");
    case "search":
      return t("approval.what.search");
    case "task":
      return t("approval.what.subagent");
    default:
      return t("approval.what.generic", { tool: toolName });
  }
}
