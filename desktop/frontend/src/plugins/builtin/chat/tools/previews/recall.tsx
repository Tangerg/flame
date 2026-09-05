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
import { INLINE_PREVIEW_ROW_LIMIT, PreviewOverflow, TEXT_PREVIEW_CLASS } from "./previewChrome";

function MemoryRecallPreview({ tool }: ToolPreviewProps) {
  const memories = projectRecalledMemories(tool.result);
  if (memories.length === 0) {
    return (
      <div className={TEXT_PREVIEW_CLASS}>
        <PreviewPlaceholder
          status={tool.status}
          pending="tools.preview.pending.recalling"
          idle="tools.preview.idle.noMemories"
        />
      </div>
    );
  }
  return (
    <div className={TEXT_PREVIEW_CLASS}>
      {memories.slice(0, INLINE_PREVIEW_ROW_LIMIT).map((memory, i) => (
        <div
          key={i}
          className="flex gap-2.5 rounded-2xs px-1 py-0.5 transition-colors hover:bg-hover"
        >
          <span className="shrink-0 tabular-nums text-fg-faint">{i + 1}</span>
          <span className="min-w-0 whitespace-pre-wrap break-words text-fg-soft">{memory}</span>
        </div>
      ))}
      <PreviewOverflow count={memories.length - INLINE_PREVIEW_ROW_LIMIT} />
    </div>
  );
}

function ConversationRecallPreview({ tool }: ToolPreviewProps) {
  const hits = projectConversationHits(tool.result);
  if (hits.length === 0) {
    return (
      <div className={TEXT_PREVIEW_CLASS}>
        <PreviewPlaceholder
          status={tool.status}
          pending="tools.preview.pending.recalling"
          idle="tools.preview.idle.noConversations"
        />
      </div>
    );
  }
  return (
    <div className={TEXT_PREVIEW_CLASS}>
      {hits.slice(0, INLINE_PREVIEW_ROW_LIMIT).map((hit, i) => (
        <div
          key={i}
          className="grid grid-cols-[minmax(0,9.5rem)_minmax(0,1fr)] gap-3 rounded-2xs px-1 py-0.5 transition-colors hover:bg-hover"
        >
          {/* The pair used to truncate as one string, and what it cut was the DAY —
              "user · 2026-0…", "assistant · 2…", two rows not even agreeing on where they
              stopped. Split, the date is `shrink-0` and the speaker gives way instead.
              The column is 9.5rem rather than the 11.13 that would fit "assistant" whole:
              measured, that is 26% of the row spent on metadata, and the speaker is a
              two-value enum whose first four characters already tell them apart. What must
              never be lost is the date, and it no longer is. Each row is its own grid, so the
              width has to be a literal for the columns to line up at all. */}
          <span className="flex min-w-0 items-baseline gap-1 text-fg-faint">
            <span className="min-w-0 truncate">{hit.speaker}</span>
            <span className="shrink-0">· {hit.day}</span>
          </span>
          <span className="min-w-0 truncate text-fg-soft">{hit.snippet}</span>
        </div>
      ))}
      <PreviewOverflow count={hits.length - INLINE_PREVIEW_ROW_LIMIT} />
    </div>
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
