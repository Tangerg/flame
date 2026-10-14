import { Badge, Button } from "@flame/ui";
import { useT } from "@/lib/i18n";
import { retrySteerInput } from "@/plugins/builtin/agent/public/input";
import type { SteerInputFeedback } from "@/plugins/builtin/agent/public/conversation";

const LABELS = {
  pending: "agent.steer.pending",
  unknown: "agent.command.unknown",
  accepted: "agent.steer.accepted",
  applied: "agent.steer.applied",
  notApplied: "agent.steer.notApplied",
  retired: "agent.steer.retired",
} as const satisfies Record<SteerInputFeedback, string>;

export function SteerInputStatus({
  feedback,
  messageId,
}: {
  feedback?: SteerInputFeedback;
  messageId: string;
}) {
  const t = useT();
  if (!feedback) return null;
  return (
    <>
      <Badge>{t(LABELS[feedback])}</Badge>
      {feedback === "unknown" && (
        <Button variant="ghost" size="sm" onClick={() => retrySteerInput(messageId)}>
          {t("agent.command.retry")}
        </Button>
      )}
    </>
  );
}
