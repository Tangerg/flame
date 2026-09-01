import type { ToolPreviewProps } from "@/plugins/sdk";
import { PreviewFoot } from "@/plugins/builtin/chat/tools/public/previews/PreviewFoot";
import { ToolOutputPanel } from "@/plugins/builtin/chat/tools/public/previews/ToolOutputPanel";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { toolPreviews } from "@/plugins/builtin/chat/tools/application/toolPreviewContributions";

function TerminalResult({ tool, onOpenView }: ToolPreviewProps) {
  return (
    <div>
      <ToolOutputPanel
        output={tool.result}
        status={tool.status}
        idleLabel="tools.preview.idle.noOutput"
      />
      <PreviewFoot label="tools.preview.openTerminal" onClick={onOpenView} />
    </div>
  );
}

function ShellCommandPreview(props: ToolPreviewProps) {
  return <TerminalResult {...props} />;
}

function ShellOutputPreview(props: ToolPreviewProps) {
  return <TerminalResult {...props} />;
}

function StopShellPreview(props: ToolPreviewProps) {
  return <TerminalResult {...props} />;
}

export const shellPreview = definePlugin({
  name: "flame.builtin.shell",
  setup(ctx) {
    // Backgrounding is an ARGUMENT of `shell` (run_in_background), not a tool of its own —
    // read_shell_output / stop_shell are how you then read and stop it.
    for (const preview of toolPreviews({
      shell: ShellCommandPreview,
      read_shell_output: ShellOutputPreview,
      stop_shell: StopShellPreview,
    })) {
      ctx.contribute(TOOL_PREVIEW, preview.component, { key: preview.key });
    }
  },
});
