import * as stylex from "@stylexjs/stylex";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { useT } from "@/lib/i18n";
import { vocab } from "@/ui";

export function PreviewPlaceholder({
  status,
  pending,
  idle,
}: {
  status: ToolCall["status"];
  pending: string;
  idle: string;
}) {
  const t = useT();
  return <div {...stylex.props(vocab.faint)}>{t(status === "running" ? pending : idle)}</div>;
}
