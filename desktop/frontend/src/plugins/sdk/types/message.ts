// Message plugin surface.

/** Built-in roles are `user`, `assistant` and `system`; a plugin may register more. */
export interface MessageRoleSpec {
  /** Stable id — matches `Message.role`. */
  id: string;
  /** Header label shown next to the timestamp — a catalog key, resolved where it
   *  renders (see `CommandSpec.label` for why it isn't resolved text). */
  displayName: string;
  icon?: string;
  /** An Avatar variant; a plugin may reuse a built-in one or fall back to default styling. */
  avatarVariant?: "msg-user" | "msg-agent" | string;
}
