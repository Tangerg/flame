import { type NotificationFeed } from "@/features/notifications/public/notifications";
import { defaultToolIconContributions } from "@/features/agent/public/toolIcon";
import { definePlugin } from "@/extensions/definePlugin";
import { TOOL_ACTION, TOOL_ICON } from "@/features/chat/public/tools/types";
import { copyToolArgsAction } from "@/features/chat/tools/application/toolActions";
import { writeToClipboard } from "@/lib/clipboard";
import { t } from "@/lib/i18n/index";
export function createToolActions(notifications: NotificationFeed) {
  return definePlugin({
    name: "flame.builtin.tool-actions",
    setup(ctx) {
      ctx.contributions.add(
        TOOL_ACTION,
        copyToolArgsAction({
          title: "toolAction.copyArguments",
          copyText: (text) =>
            writeToClipboard(notifications.show, text, {
              successLabel: t("toolAction.argumentsCopied"),
              failureLabel: t("toolAction.copyFailed", { section: t("toolInspector.arguments") }),
            }),
        }),
      );
    },
  });
}

export const toolIcons = definePlugin({
  name: "flame.builtin.tool-icons",
  setup(ctx) {
    for (const { key, icon } of defaultToolIconContributions()) {
      ctx.contributions.add(TOOL_ICON, icon, { key });
    }
  },
});
