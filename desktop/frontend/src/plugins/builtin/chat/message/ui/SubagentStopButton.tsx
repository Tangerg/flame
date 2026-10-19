import { IconButton } from "@flame/ui";
import { useT } from "@/lib/i18n";
import { cancelSessionRun } from "@/plugins/builtin/agent/public/run";
import { useRuntimeCommandsAvailable } from "@/plugins/builtin/runtime/public/serviceStatus";
import type { AgentRunView } from "@/plugins/sdk/types/agentSessionView";
import { iconGlyph } from "@/ui/icons";

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
