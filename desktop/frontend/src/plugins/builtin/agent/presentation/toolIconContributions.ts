import { TOOL_ICON_BY_NAME } from "@/lib/toolFamilies";

export interface ToolIconContribution {
  key: string;
  icon: string;
}

export function defaultToolIconContributions(): ToolIconContribution[] {
  return Object.entries(TOOL_ICON_BY_NAME).map(([key, icon]) => ({ key, icon }));
}

export function defaultToolIconFor(key: string): string {
  return TOOL_ICON_BY_NAME[key] ?? "tool";
}
