import * as stylex from "@stylexjs/stylex";
import type { ToolPreviewProps } from "@/plugins/sdk";
import { PreviewPlaceholder } from "@/plugins/builtin/chat/tools/public/previews/PreviewPlaceholder";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { MarkdownMessage } from "./ui/markdown/MarkdownMessage";
import { face, leading, space, type as typeStep } from "@/styles/tokens.stylex";
import { vocab } from "@/ui";

const styles = stylex.create({
  reply: {
    maxHeight: "min(50vh, 20lh)",
    overflowY: "auto",
    overflowWrap: "anywhere",
    paddingTop: space.s1,
    lineHeight: leading.body,
  },
});

function TaskPreview({ tool }: ToolPreviewProps) {
  if (!tool.result?.trim()) {
    return (
      <PreviewPlaceholder
        status={tool.status}
        pending="tools.preview.pending.delegating"
        idle="tools.preview.idle.noReply"
      />
    );
  }
  return (
    <div
      role="region"
      aria-label={tool.fn}
      // oxlint-disable-next-line jsx-a11y/no-noninteractive-tabindex -- Native scrolling must be keyboard-accessible independently of the transcript.
      tabIndex={0}
      {...stylex.props(styles.reply, face.text, typeStep.prose, vocab.soft, vocab.min)}
    >
      <MarkdownMessage text={tool.result} reveal="instant" />
    </div>
  );
}

export const taskPreview = definePlugin({
  name: "flame.builtin.task-preview",
  setup(ctx) {
    ctx.contribute(TOOL_PREVIEW, TaskPreview, { key: "delegate_task" });
  },
});
