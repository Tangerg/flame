import * as stylex from "@stylexjs/stylex";
import type { ToolPreviewProps } from "@/plugins/sdk";
import { Badge, vocab } from "@/ui";
import { PreviewPlaceholder } from "@/plugins/builtin/chat/tools/public/previews/PreviewPlaceholder";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { projectToolSearchGroups } from "@/plugins/builtin/chat/tools/application/specialisedPreviewProjections";
import { toolPreviews } from "@/plugins/builtin/chat/tools/application/toolPreviewContributions";

import { space, type as typeStep } from "@/styles/tokens.stylex";
import { previewStyles as pv } from "./previewStyles";
import { TEXT_PREVIEW } from "./previewChrome";

const ts = stylex.create({
  // A tool search can return dozens: the list scrolls rather than pushing the transcript.
  scroller: { maxHeight: "calc(var(--spacing) * 60)", overflowY: "auto" },
  group: { display: "flex", alignItems: "flex-start", gap: space.s2_5, paddingBlock: space.s1 },
  // One measure for every source name, so the chips beside them start on one line.
  source: { width: "calc(var(--spacing) * 20)", flexShrink: 0, paddingTop: space.s0_5 },
  chips: { display: "flex", minWidth: 0, flexWrap: "wrap", gap: space.s1 },
});

function ToolSearchPreview({ tool }: ToolPreviewProps) {
  const groups = projectToolSearchGroups(tool.result);
  if (groups.length === 0) {
    return (
      <div {...stylex.props(TEXT_PREVIEW)}>
        <PreviewPlaceholder
          status={tool.status}
          pending="tools.preview.pending.loadingTools"
          idle="tools.preview.idle.noTools"
        />
      </div>
    );
  }
  return (
    <div {...stylex.props(ts.scroller, pv.inset)}>
      {groups.map((group) => (
        <div key={group.source} {...stylex.props(ts.group)}>
          <span {...stylex.props(ts.source, vocab.truncate, vocab.faint, typeStep.uiSm)}>
            {group.source}
          </span>
          <div {...stylex.props(ts.chips)}>
            {group.names.map((name) => (
              <Badge key={name} face="mono">
                {name}
              </Badge>
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}

export const toolSearchPreviewPlugin = definePlugin({
  name: "flame.builtin.tool-search-preview",
  setup(ctx) {
    for (const preview of toolPreviews({ search_tools: ToolSearchPreview })) {
      ctx.contribute(TOOL_PREVIEW, preview.component, { key: preview.key });
    }
  },
});
