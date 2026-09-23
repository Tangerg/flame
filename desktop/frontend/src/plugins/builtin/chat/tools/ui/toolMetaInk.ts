import type { ToolMetaTone } from "@/plugins/builtin/agent/public/messagePresentation";
import { vocab } from "@/ui";

export const toolMetaInk = {
  card: { muted: vocab.muted, negative: vocab.negative },
  member: { muted: vocab.faint, negative: vocab.negative },
} as const satisfies Record<"card" | "member", Record<ToolMetaTone, unknown>>;
