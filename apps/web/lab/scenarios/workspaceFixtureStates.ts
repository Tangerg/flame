import type { LabText } from "../story";

export const VISUAL_DOCK_WIDTH_RATIO = 0.5;

export const VISUAL_WORKSPACE_VIEWPORT = { width: 1472, height: 900 } as const;

export const VISUAL_REVIEW_DOCK_WIDTH_RATIO = 0.36;

export const VISUAL_REVIEW_VIEWPORT = { width: 1800, height: 1000 } as const;

export const VISUAL_SETTINGS_PANES = [
  "appearance",
  "brand",
  "personalization",
  "providers",
  "approvals",
  "mcp-servers",
  "hooks",
  "plugins",
  "connection",
  "shortcuts",
] as const;

export type VisualSettingsPane = (typeof VISUAL_SETTINGS_PANES)[number];

export function isVisualSettingsPane(value: string | null): value is VisualSettingsPane {
  return VISUAL_SETTINGS_PANES.includes(value as VisualSettingsPane);
}

export const VISUAL_WORKSPACE_STATES = [
  "dock-light",
  "dock-review",
  "dock-trajectory",
  "dock-runs",
  "dock-subagents",
  "dock-diagnostics",
  "dock-diagnostics-recorded",
  "dock-files",
  "dock-skills",
  "dock-skills-catalog",
  "dock-agent-memory",
  "dock-schedules",
  "dock-schedules-models",
  "dock-usage",
  "dock-usage-reports",
  "dock-feature-off",
  "dock-file",
  "dock-file-error",
  "dock-file-unsupported",
  "dock-empty",
  "dock-catalog",
  "dock-loading",
  "dock-error",
  "full-view",
  "settings",
] as const;

export type VisualWorkspaceState = (typeof VISUAL_WORKSPACE_STATES)[number];

export const VISUAL_WORKSPACE_STATE_TITLES: Readonly<Record<VisualWorkspaceState, LabText>> = {
  "dock-light": { en: "Files workspace", "zh-CN": "文件工作区" },
  "dock-review": { en: "Diff review", "zh-CN": "差异审阅" },
  "dock-trajectory": { en: "Session trajectory", "zh-CN": "会话轨迹" },
  "dock-runs": { en: "Run tree", "zh-CN": "运行树" },
  "dock-subagents": { en: "Subagents", "zh-CN": "子代理" },
  "dock-diagnostics": { en: "Diagnostics", "zh-CN": "诊断" },
  "dock-diagnostics-recorded": { en: "Diagnostics · recorded", "zh-CN": "诊断 · 已记录" },
  "dock-files": { en: "Files", "zh-CN": "文件" },
  "dock-skills": { en: "Skills", "zh-CN": "技能" },
  "dock-skills-catalog": { en: "Skills · catalog", "zh-CN": "技能 · 目录" },
  "dock-agent-memory": { en: "Agent memory", "zh-CN": "Agent 记忆" },
  "dock-schedules": { en: "Schedules", "zh-CN": "计划任务" },
  "dock-schedules-models": { en: "Schedules · model choices", "zh-CN": "计划任务 · 模型选择" },
  "dock-usage": { en: "Usage", "zh-CN": "用量" },
  "dock-usage-reports": { en: "Saved usage reports", "zh-CN": "已保存的用量报告" },
  "dock-feature-off": { en: "Features off", "zh-CN": "功能已关闭" },
  "dock-file": { en: "File viewer", "zh-CN": "文件阅读" },
  "dock-file-error": { en: "File read failure", "zh-CN": "文件读取失败" },
  "dock-file-unsupported": { en: "Unsupported file", "zh-CN": "不支持的文件" },
  "dock-empty": { en: "Diff · empty", "zh-CN": "差异 · 空状态" },
  "dock-catalog": { en: "Dock catalogue", "zh-CN": "面板目录" },
  "dock-loading": { en: "Diff · loading", "zh-CN": "差异 · 加载中" },
  "dock-error": { en: "Diff · error", "zh-CN": "差异 · 加载失败" },
  "full-view": { en: "Full view", "zh-CN": "全宽阅读" },
  settings: { en: "Settings", "zh-CN": "设置" },
};

export type VisualWorkspaceTheme = "light" | "dark";

export function isVisualWorkspaceState(value: string | null): value is VisualWorkspaceState {
  return VISUAL_WORKSPACE_STATES.includes(value as VisualWorkspaceState);
}

export const DOCK_VIEW_BY_STATE: Partial<Record<VisualWorkspaceState, string>> = {
  "dock-light": "file",
  "dock-review": "package:visual:diff",
  "dock-empty": "package:visual:diff",
  "dock-loading": "package:visual:diff",
  "dock-error": "package:visual:diff",
  "dock-trajectory": "package:visual:trajectory",
  "dock-runs": "package:visual:trajectory",
  "dock-subagents": "subagents",
  "dock-diagnostics": "package:visual:diagnostics",
  "dock-diagnostics-recorded": "package:visual:diagnostics",
  "dock-files": "file",
  "dock-skills": "package:visual:skills",
  "dock-skills-catalog": "package:visual:skills",
  "dock-agent-memory": "package:visual:memory",
  "dock-schedules": "package:visual:schedules",
  "dock-schedules-models": "package:visual:schedules",
  "dock-usage": "package:visual:usage",
  "dock-usage-reports": "package:visual:usage-reports",
  "dock-feature-off": "package:visual:skills",
  "dock-file": "file",
  "dock-file-error": "file",
  "dock-file-unsupported": "file",
};
