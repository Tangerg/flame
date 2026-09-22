import * as stylex from "@stylexjs/stylex";
import { TextPreview } from "@/ui";
import type { ToolPreviewProps } from "@/plugins/sdk";
import { PreviewPlaceholder } from "@/plugins/builtin/chat/tools/public/previews/PreviewPlaceholder";
import { SearchResults } from "@/plugins/builtin/chat/tools/public/previews/SearchResults";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { projectWebSearchPreview } from "@/plugins/builtin/chat/tools/application/specialisedPreviewProjections";
import { toolPreviews } from "@/plugins/builtin/chat/tools/application/toolPreviewContributions";
import { toolShapeKey } from "@/plugins/builtin/agent/public/toolIcon";
import { PreviewOverflow } from "./previewChrome";
import { previewStyles as pv } from "./previewStyles";

const MAX_WEB_RESULTS = 8;

function WebSearchPreview({ tool }: ToolPreviewProps) {
  const results = projectWebSearchPreview(tool.result);
  if (results.length === 0) {
    return (
      <TextPreview>
        <PreviewPlaceholder
          status={tool.status}
          pending="tools.preview.pending.searching"
          idle="tools.preview.idle.noResults"
        />
      </TextPreview>
    );
  }
  return (
    <div {...stylex.props(pv.inset)}>
      <SearchResults results={results.slice(0, MAX_WEB_RESULTS)} />
      <PreviewOverflow count={results.length - MAX_WEB_RESULTS} />
    </div>
  );
}

export const webSearchPreview = definePlugin({
  name: "flame.builtin.web-search-preview",
  setup(ctx) {
    for (const preview of toolPreviews({
      web_search: WebSearchPreview,
      [toolShapeKey("webSearch")]: WebSearchPreview,
    })) {
      ctx.contribute(TOOL_PREVIEW, preview.component, { key: preview.key });
    }
  },
});
