import { toolCategory } from "@/plugins/builtin/agent/public/viewState";

export function approvalHeadline(
  t: (key: string, params?: Record<string, string | number>) => string,
  toolName: string | undefined,
): string {
  if (!toolName) return t("approval.fallbackText");
  switch (toolCategory(toolName)) {
    case "command":
      return t("approval.what.command");
    case "fileEdit":
      return t("approval.what.fileEdit");
    case "search":
      return t("approval.what.search");
    case "webSearch":
      return t("approval.what.webSearch");
    case "subagent":
      return t("approval.what.subagent");
    default:
      return t("approval.what.generic", { tool: toolName });
  }
}
