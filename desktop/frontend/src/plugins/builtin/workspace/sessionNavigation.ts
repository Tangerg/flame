import { definePlugin } from "@/plugins/sdk";
import { AGENT_SESSIONS } from "@/plugins/builtin/agent/public/services";
import { WORKSPACE_SCOPE } from "@/plugins/builtin/workspace/public/services";
import { bindWorkspaceSessionNavigation } from "./application/sessionNavigationSync";

export const workspaceSessionNavigation = definePlugin({
  name: "flame.builtin.workspace.session-navigation",
  requires: { sessions: AGENT_SESSIONS, scopes: WORKSPACE_SCOPE },
  setup(ctx) {
    ctx.cleanup(bindWorkspaceSessionNavigation({ ...ctx.sessions, ...ctx.scopes }));
  },
});
