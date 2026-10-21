import { defaultToolIconContributions } from "@/features/agent/public/toolIcon";
import { definePlugin } from "@/extensions/definePlugin";
import { TOOL_ACTION, TOOL_ICON } from "@/features/chat/public/tools/types";
import { copyToolArgsAction } from "@/features/chat/tools/application/toolActions";
import { writeToClipboard } from "@/lib/clipboard";
import { t } from "@/lib/i18n/index";
export const toolActions = definePlugin({
  name: "flame.builtin.tool-actions",
  setup(ctx) {
    ctx.contribute(
      TOOL_ACTION,
      copyToolArgsAction({
        title: "toolAction.copyArguments",
        copyText: (text) =>
          writeToClipboard(text, {
            successLabel: t("toolAction.argumentsCopied"),
            failureLabel: t("toolAction.copyFailed", { section: t("toolInspector.arguments") }),
          }),
      }),
    );
  },
});

export const toolIcons = definePlugin({
  name: "flame.builtin.tool-icons",
  setup(ctx) {
    for (const { key, icon } of defaultToolIconContributions()) {
      ctx.contribute(TOOL_ICON, icon, { key });
    }
  },
});
