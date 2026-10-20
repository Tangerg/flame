import { type MessageRoleSpec } from "@/features/chat/public/messageRoles";
import { definePlugin } from "@/extensions/definePlugin";
import { MESSAGE_ROLE } from "@/features/chat/public/messageRoles";
export function defaultMessageRoles(): MessageRoleSpec[] {
  return [
    {
      id: "user",
      displayName: "role.user",
      icon: "user",
      avatarVariant: "msg-user",
    },
    {
      id: "assistant",
      displayName: "role.assistant",
      icon: "spark",
      avatarVariant: "msg-agent",
    },
    {
      id: "system",
      displayName: "role.system",
      icon: "shield",
      avatarVariant: "msg-agent",
    },
  ];
}

export const defaultRoles = definePlugin({
  name: "flame.builtin.default-roles",
  setup(ctx) {
    for (const role of defaultMessageRoles()) {
      ctx.contribute(MESSAGE_ROLE, role);
    }
  },
});
