import * as stylex from "@stylexjs/stylex";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { useT } from "@/lib/i18n";
import { chatStyles as ct } from "../../../chatStyles";

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
  return <div {...stylex.props(ct.faint)}>{t(status === "running" ? pending : idle)}</div>;
}
