import * as stylex from "@stylexjs/stylex";
import type { ToolPreviewProps } from "@/plugins/sdk";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { recordedReadLines } from "@/plugins/builtin/chat/tools/application/toolPreviewRecords";
import { useT } from "@/lib/i18n";
import { toolPreviews } from "@/plugins/builtin/chat/tools/application/toolPreviewContributions";

import { type as typeStep } from "@/styles/tokens.stylex";
import { previewStyles as pv } from "./previewStyles";
import { TextPreview, vocab } from "@/ui";

const MAX_FILE_LINES = 40;

function FilePreview({ tool }: ToolPreviewProps) {
  const t = useT();
  if (tool.status === "running") return null;
  const recorded = recordedReadLines(tool, MAX_FILE_LINES);
  if (!recorded) {
    return (
      <TextPreview>
        <p {...stylex.props(pv.note, vocab.faint, typeStep.uiSm)}>{t("tools.read.notRecorded")}</p>
      </TextPreview>
    );
  }
  return (
    <TextPreview>
      <div {...stylex.props(pv.sheet, typeStep.uiSm)}>
        {recorded.lines.map((l) => (
          <div key={l.lineNumber} {...stylex.props(pv.numbered, pv.row)}>
            <span {...stylex.props(pv.gutter, typeStep.uiSm)}>{l.lineNumber}</span>
            <span {...stylex.props(pv.wrap, vocab.soft)}>{l.text || " "}</span>
          </div>
        ))}
        {recorded.hidden > 0 && (
          <div {...stylex.props(vocab.faint)}>
            … {t("tools.overflow.lines", { count: recorded.hidden })}
          </div>
        )}
      </div>
    </TextPreview>
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
