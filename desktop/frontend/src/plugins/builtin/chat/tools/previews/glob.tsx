import * as stylex from "@stylexjs/stylex";
import type { ToolPreviewProps } from "@/plugins/sdk";
import { PreviewPlaceholder } from "@/plugins/builtin/chat/tools/public/previews/PreviewPlaceholder";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { projectGlobPreview } from "@/plugins/builtin/chat/tools/application/specialisedPreviewProjections";
import { toolPreviews } from "@/plugins/builtin/chat/tools/application/toolPreviewContributions";
import { INLINE_PREVIEW_ROW_LIMIT, PreviewOverflow } from "./previewChrome";
import { previewStyles as pv } from "./previewStyles";
import { TextPreview, vocab } from "@/ui";

function GlobPreview({ tool }: ToolPreviewProps) {
  const { paths } = projectGlobPreview(tool.result);
  return (
    <TextPreview>
      {paths.length === 0 && (
        <PreviewPlaceholder
          status={tool.status}
          pending="tools.preview.pending.matching"
          idle="tools.preview.idle.noMatches"
        />
      )}
      {paths.slice(0, INLINE_PREVIEW_ROW_LIMIT).map((p) => (
        <div
          key={p}
          className={stylex.props(vocab.truncate, pv.row, pv.rowPad, vocab.muted).className}
        >
          {p}
        </div>
      ))}
      <PreviewOverflow count={paths.length - INLINE_PREVIEW_ROW_LIMIT} />
    </TextPreview>
  );
}

export const globPreview = definePlugin({
  name: "flame.builtin.glob-preview",
  setup(ctx) {
    for (const preview of toolPreviews({ glob: GlobPreview })) {
      ctx.contribute(TOOL_PREVIEW, preview.component, { key: preview.key });
    }
  },
});
