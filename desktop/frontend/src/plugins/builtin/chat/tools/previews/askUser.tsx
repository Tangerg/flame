import * as stylex from "@stylexjs/stylex";
import type { ToolPreviewProps } from "@/plugins/sdk";
import { useT } from "@/lib/i18n";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { projectAskUserAnswer } from "@/plugins/builtin/chat/tools/application/specialisedPreviewProjections";
import { toolPreviews } from "@/plugins/builtin/chat/tools/application/toolPreviewContributions";

import { chatStyles as ct } from "../../chatStyles";
import { TEXT_PREVIEW } from "./previewChrome";
import { previewStyles as pv } from "./previewStyles";

function AskUserPreview({ tool }: ToolPreviewProps) {
  const t = useT();
  const answer = projectAskUserAnswer(tool.result);
  return (
    <div {...stylex.props(TEXT_PREVIEW, pv.wrapWords)}>
      {answer ? (
        <>
          <span {...stylex.props(ct.faint)}>{t("tool.askUser.answerPrefix")}</span>
          <span {...stylex.props(ct.soft)}>{answer}</span>
        </>
      ) : (
        <span {...stylex.props(ct.faint)}>{t("tool.askUser.waiting")}</span>
      )}
    </div>
  );
}

export const askUserPreview = definePlugin({
  name: "flame.builtin.ask-user-preview",
  setup(ctx) {
    for (const preview of toolPreviews({ ask_user: AskUserPreview })) {
      ctx.contribute(TOOL_PREVIEW, preview.component, { key: preview.key });
    }
  },
});
