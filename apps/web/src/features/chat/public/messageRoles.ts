import { defineExtensionPoint } from "@/extensions/contracts";
export interface MessageRoleSpec {
  id: string;
  displayName: string;
  icon?: string;
  avatarVariant?: "msg-user" | "msg-agent" | string;
}

export const MESSAGE_ROLE = defineExtensionPoint<MessageRoleSpec>({
  id: "flame.message.role",
  keying: "single",
});
