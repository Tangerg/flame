export interface MessageRoleSpec {
  id: string;
  displayName: string;
  icon?: string;
  avatarVariant?: "msg-user" | "msg-agent" | string;
}
