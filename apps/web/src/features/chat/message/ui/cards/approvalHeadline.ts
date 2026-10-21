import { type ToolCategory } from "@flame/runtime-client/wire";
export function approvalHeadline(
  t: (key: string, params?: Record<string, string | number>) => string,
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
    case "web":
      return t("approval.what.webSearch");
    case "task":
      return t("approval.what.subagent");
    default:
      return t("approval.what.generic", { tool: toolName });
  }
}
