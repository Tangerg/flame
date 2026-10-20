import { IconButton } from "@flame/ui";
import { useT } from "@/lib/i18n/index";
import { cancelSessionRun } from "@/features/agent/public/run";
import { useRuntimeCommandsAvailable } from "@/features/connection/public/serviceStatus";
import { type AgentRunView } from "@/features/agent/public/sessionView";
import { iconGlyph } from "@/ui/icons/index";
export function SubagentStopButton({ run }: { run: Pick<AgentRunView, "id" | "sessionId"> }) {
  const t = useT();
  const available = useRuntimeCommandsAvailable();
  return (
    <IconButton
      icon={iconGlyph("stop")}
      size="sm"
      variant="soft"
      round
      disabled={!available}
      aria-label={t("agent.runTree.action.cancel")}
      onClick={() => cancelSessionRun({ sessionId: run.sessionId, runId: run.id })}
    />
  );
}
