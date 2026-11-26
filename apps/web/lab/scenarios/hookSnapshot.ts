import { type HooksListResult } from "@flame/runtime-client/wire";

export const VISUAL_HOOKS: HooksListResult = {
  repositoryRoot: "/Users/visual/scope",
  repositoryTrusted: false,
  hooks: [
    {
      event: "PreToolUse",
      scope: "global",
      source: "/Users/visual/.flame/hooks.json",
      matcher: "Bash",
      command: "~/.flame/hooks/deny-force-push.sh",
      failurePolicy: "required",
      active: true,
    },
    {
      event: "PostToolUse",
      scope: "global",
      source: "/Users/visual/.flame/hooks.json",
      matcher: "Edit|Write",
      command: 'npx prettier --write "$FLAME_TOOL_PATH"',
      failurePolicy: "advisory",
      active: true,
    },
    {
      event: "SessionStart",
      scope: "repository",
      source: "/Users/visual/scope/.flame/hooks.json",
      inject: "Read DEVELOPMENT.md before changing a module.",
      active: false,
    },
    {
      event: "Stop",
      scope: "repository",
      source: "/Users/visual/scope/.flame/hooks.json",
      command: "make check-quick",
      failurePolicy: "advisory",
      active: false,
    },
  ],
};
