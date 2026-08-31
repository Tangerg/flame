import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { useT } from "@/lib/i18n";

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
  return <div className="text-fg-faint">{t(status === "running" ? pending : idle)}</div>;
}
