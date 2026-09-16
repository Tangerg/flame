import type { ToolPreviewProps } from "@/plugins/sdk";
import { ToolOutputPanel } from "@/plugins/builtin/chat/tools/public/previews/ToolOutputPanel";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { toolPreviews } from "@/plugins/builtin/chat/tools/application/toolPreviewContributions";
import { toolShapeKey } from "@/plugins/builtin/chat/tools/public/toolIcon";
import { commandToolResult } from "@/plugins/sdk";

function ShellOutput({ tool }: ToolPreviewProps) {
  return (
    <div>
      <ToolOutputPanel
        output={tool.result}
        status={tool.status}
        idleLabel="tools.preview.idle.noOutput"
      />
    </div>
  );
}

function CommandShapePreview({ tool }: ToolPreviewProps) {
  return (
    <div>
      <ToolOutputPanel
        output={commandToolResult(tool.result)?.output}
        status={tool.status}
        idleLabel="tools.preview.idle.noOutput"
      />
    </div>
  );
}

export const shellPreview = definePlugin({
  name: "flame.builtin.shell",
  setup(ctx) {
    // Backgrounding is an ARGUMENT of `shell` (run_in_background), not a tool of its own —
    // read_shell_output / stop_shell are how you then read and stop it.
    for (const preview of toolPreviews({
      shell: ShellOutput,
      read_shell_output: ShellOutput,
      stop_shell: ShellOutput,
      [toolShapeKey("command")]: CommandShapePreview,
    })) {
      ctx.contribute(TOOL_PREVIEW, preview.component, { key: preview.key });
    }
  },
});
