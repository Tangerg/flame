import { definePlugin } from "@/extensions/definePlugin";
import { AGENT_SESSIONS } from "@/features/agent/public/services";
import { WORKSPACE_SCOPE } from "@/features/workspace/public/services";
import { bindWorkspaceSessionNavigation } from "@/features/workspace/application/sessionNavigationSync";
export const workspaceSessionNavigation = definePlugin({
  name: "flame.builtin.workspace.session-navigation",
  requires: { sessions: AGENT_SESSIONS, scopes: WORKSPACE_SCOPE },
  setup(ctx) {
    ctx.cleanup(bindWorkspaceSessionNavigation({ ...ctx.sessions, ...ctx.scopes }));
  },
});
