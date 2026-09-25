import * as stylex from "@stylexjs/stylex";
import type { ToolPreviewProps } from "@/plugins/sdk";
import { PreviewPlaceholder } from "@/plugins/builtin/chat/tools/public/previews/PreviewPlaceholder";
import { ToolOutputPanel } from "@/plugins/builtin/chat/tools/public/previews/ToolOutputPanel";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { projectRecalledMemories } from "@/plugins/builtin/chat/tools/application/specialisedPreviewProjections";
import { toolPreviews } from "@/plugins/builtin/chat/tools/application/toolPreviewContributions";
import { INLINE_PREVIEW_ROW_LIMIT, PreviewOverflow } from "./previewChrome";
import { space } from "@/styles/tokens.stylex";
import { previewStyles as pv } from "./previewStyles";
import { TextPreview, vocab } from "@/ui";

const rc = stylex.create({
  memory: { display: "flex", gap: space.s2_5 },
});

function MemoryRecallPreview({ tool }: ToolPreviewProps) {
  const memories = projectRecalledMemories(tool.result);
  if (memories.length === 0) {
    return (
      <TextPreview>
        <PreviewPlaceholder
          status={tool.status}
          pending="tools.preview.pending.recalling"
          idle="tools.preview.idle.noMemories"
        />
      </TextPreview>
    );
  }
  return (
    <TextPreview>
      {memories.slice(0, INLINE_PREVIEW_ROW_LIMIT).map((memory, i) => (
        <div key={i} className={stylex.props(rc.memory, pv.row, pv.rowPad).className}>
          <span {...stylex.props(vocab.hold, vocab.figures, vocab.faint)}>{i + 1}</span>
          <span {...stylex.props(pv.wrapWords, vocab.soft)}>{memory}</span>
        </div>
      ))}
      <PreviewOverflow count={memories.length - INLINE_PREVIEW_ROW_LIMIT} />
    </TextPreview>
  );
}

function StoredToolResultPreview({ tool }: ToolPreviewProps) {
  return (
    <ToolOutputPanel
      output={tool.result}
      status={tool.status}
      idleLabel="tools.preview.idle.noOutput"
    />
  );
}

export const recallPreviews = definePlugin({
  name: "flame.builtin.recall-previews",
  setup(ctx) {
    for (const preview of toolPreviews({
      search_memory: MemoryRecallPreview,
      read_tool_result: StoredToolResultPreview,
    })) {
      ctx.contribute(TOOL_PREVIEW, preview.component, { key: preview.key });
    }
  },
});
