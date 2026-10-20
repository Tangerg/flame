import { defaultToolIconContributions } from "@/features/agent/public/toolIcon";
import { copyText } from "@flame/ui";
import { definePlugin } from "@/extensions/definePlugin";
import { TOOL_ACTION, TOOL_ICON } from "@/features/chat/public/tools/types";
import { copyToolArgsAction } from "@/features/chat/tools/application/toolActions";
export const toolActions = definePlugin({
  name: "flame.builtin.tool-actions",
  setup(ctx) {
    ctx.contribute(
      TOOL_ACTION,
      copyToolArgsAction({ title: "toolAction.copyArguments", copyText }),
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
