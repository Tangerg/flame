import * as stylex from "@stylexjs/stylex";
import type { ToolPreviewProps } from "@/plugins/sdk";
import { PreviewFoot } from "@/plugins/builtin/chat/tools/public/previews/PreviewFoot";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { useFileToolPreview } from "@/plugins/builtin/chat/tools/application/toolPreviewQueries";
import { toolPreviews } from "@/plugins/builtin/chat/tools/application/toolPreviewContributions";

import { type as typeStep } from "@/styles/tokens.stylex";
import { chatStyles as ct } from "../../chatStyles";
import { previewStyles as pv } from "./previewStyles";
import { TEXT_PREVIEW } from "./previewChrome";

const MAX_FILE_LINES = 40;

function FilePreview({ tool, onOpenView }: ToolPreviewProps) {
  const { data: lines } = useFileToolPreview(tool, MAX_FILE_LINES);
  return (
    <div {...stylex.props(TEXT_PREVIEW)}>
      <div {...stylex.props(pv.sheet, typeStep.uiSm)}>
        {(lines ?? []).map((l) => (
          <div key={l.lineNumber} {...stylex.props(pv.numbered, pv.row)}>
            <span {...stylex.props(pv.gutter, typeStep.uiSm)}>{l.lineNumber}</span>
            <span {...stylex.props(pv.wrap, ct.soft)}>{l.text || " "}</span>
          </div>
        ))}
      </div>
      <PreviewFoot label="tools.preview.viewFile" onClick={onOpenView} />
    </div>
  );
}

export const file = definePlugin({
  name: "flame.builtin.file",
  setup(ctx) {
    for (const preview of toolPreviews({ read: FilePreview })) {
      ctx.contribute(TOOL_PREVIEW, preview.component, { key: preview.key });
    }
  },
});
