import { type ListApprovalRulesResult } from "@flame/runtime-client/wire";

export const VISUAL_APPROVAL_RULES: ListApprovalRulesResult = {
  rules: [
    {
      id: "rule-session-status",
      scope: "session",
      decision: "allow",
      modelName: "shell",
      tool: { type: "builtIn", name: "shell" },
      subject: { type: "glob", value: "git status*" },
      stale: false,
    },
    {
      id: "rule-folder-edit",
      scope: "workspace",
      dir: "/Users/visual/scope",
      decision: "allow",
      modelName: "edit",
      tool: { type: "builtIn", name: "edit" },
      subject: { type: "all" },
      stale: false,
    },
    {
      id: "rule-global-force-push",
      scope: "global",
      decision: "deny",
      modelName: "shell",
      tool: { type: "builtIn", name: "shell" },
      subject: { type: "glob", value: "git push --force*" },
      stale: false,
    },
    {
      id: "rule-global-issues",
      scope: "global",
      decision: "allow",
      modelName: "linear_list_issues",
      tool: {
        type: "mcp",
        name: "list_issues",
        server: { name: "linear", origin: { type: "user" } },
      },
      subject: { type: "all" },
      stale: true,
    },
  ],
};
