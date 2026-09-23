import type { WorkspaceViewSpec } from "@/plugins/sdk";
import type { AnyPlugin } from "dougong";
import { definePlugin } from "@/plugins/sdk";
import { WORKSPACE_VIEW } from "@/plugins/sdk/kernelPoints";

export function defineWorkspaceView(spec: WorkspaceViewSpec): AnyPlugin {
  return definePlugin({
    name: `flame.builtin.view-${spec.id}`,
    setup(ctx) {
      ctx.contribute(WORKSPACE_VIEW, spec);
    },
  });
}
