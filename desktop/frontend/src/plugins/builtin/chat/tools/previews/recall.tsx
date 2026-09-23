import * as stylex from "@stylexjs/stylex";
import type { ToolPreviewProps } from "@/plugins/sdk";
import { PreviewPlaceholder } from "@/plugins/builtin/chat/tools/public/previews/PreviewPlaceholder";
import { ToolOutputPanel } from "@/plugins/builtin/chat/tools/public/previews/ToolOutputPanel";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import {
  projectConversationHits,
  projectRecalledMemories,
} from "@/plugins/builtin/chat/tools/application/specialisedPreviewProjections";
import { toolPreviews } from "@/plugins/builtin/chat/tools/application/toolPreviewContributions";
import { INLINE_PREVIEW_ROW_LIMIT, PreviewOverflow } from "./previewChrome";
import { space } from "@/styles/tokens.stylex";
import { previewStyles as pv } from "./previewStyles";
import { TextPreview, vocab } from "@/ui";

const rc = stylex.create({
  memory: { display: "flex", gap: space.s2_5 },
  // The speaker column holds a measure so the snippets beside them start level.
  hit: { display: "grid", gridTemplateColumns: "minmax(0, 9.5rem) minmax(0, 1fr)", gap: space.s3 },
  who: { display: "flex", minWidth: 0, alignItems: "baseline", gap: space.s1 },
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

function ConversationRecallPreview({ tool }: ToolPreviewProps) {
  const hits = projectConversationHits(tool.result);
  if (hits.length === 0) {
    return (
      <TextPreview>
        <PreviewPlaceholder
          status={tool.status}
          pending="tools.preview.pending.recalling"
          idle="tools.preview.idle.noConversations"
        />
      </TextPreview>
    );
  }
  return (
    <TextPreview>
      {hits.slice(0, INLINE_PREVIEW_ROW_LIMIT).map((hit, i) => (
        <div key={i} className={stylex.props(rc.hit, pv.row, pv.rowPad).className}>
          {/* The date never shrinks and the speaker gives way: the speaker is a two-value enum
              whose first four characters tell them apart, while a cut date is lost. 9.5rem and
              not the 11.13 that fits "assistant" whole, which spends a quarter of the row on
              metadata. Each row is its own grid, so the width is a literal for the columns to
              line up. */}
          <span {...stylex.props(rc.who, vocab.faint)}>
            <span {...stylex.props(vocab.min, vocab.truncate)}>{hit.speaker}</span>
            <span {...stylex.props(vocab.hold)}>· {hit.day}</span>
          </span>
          <span {...stylex.props(vocab.min, vocab.truncate, vocab.soft)}>{hit.snippet}</span>
        </div>
      ))}
      <PreviewOverflow count={hits.length - INLINE_PREVIEW_ROW_LIMIT} />
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
    // Searching the agent's own history: project memory and earlier conversations. Two
    // shapes, one family — both answer "here is what I already knew".
    for (const preview of toolPreviews({
      search_memory: MemoryRecallPreview,
      search_conversations: ConversationRecallPreview,
      read_tool_result: StoredToolResultPreview,
    })) {
      ctx.contribute(TOOL_PREVIEW, preview.component, { key: preview.key });
    }
  },
});
