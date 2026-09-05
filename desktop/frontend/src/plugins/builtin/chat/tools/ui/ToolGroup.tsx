import { useState } from "react";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { toolIconFor } from "@/plugins/builtin/chat/tools/public/toolIcon";
import { AgentActivityDisclosure } from "@/ui/agent";
import { useT } from "@/lib/i18n";
import { toolGroupModel, type ToolGroupPinnedState } from "../application/toolGroupModel";
import { ToolGroupMember } from "./ToolGroupMember";

interface Props {
  tools: ToolCall[];
  onSelectTool: (id: string) => void;
  expandedIds: Set<string>;
  onToggleExpand: (id: string) => void;
  superseded?: boolean;
}

export function ToolGroup({ tools, onSelectTool, expandedIds, onToggleExpand, superseded }: Props) {
  const [pinned, setPinned] = useState<ToolGroupPinnedState>(null);
  const t = useT();
  const model = toolGroupModel(t, tools, pinned, superseded);

  return (
    <AgentActivityDisclosure
      icon={toolIconFor(model.dominantTool)}
      shell="line"
      label={model.summary}
      trailing={
        <span className="font-mono text-ui-xs font-medium text-fg-muted">
          {t("tools.group.calls", { count: model.count })}
        </span>
      }
      open={model.expanded}
      onToggle={() => setPinned(model.nextPinned)}
      stickyHeader
    >
      <div className="flex flex-col gap-1">
        {tools.map((tool) => (
          <ToolGroupMember
            key={tool.id}
            tool={tool}
            expanded={expandedIds.has(tool.id)}
            onToggleExpand={() => {
              onSelectTool(tool.id);
              onToggleExpand(tool.id);
            }}
          />
        ))}
      </div>
    </AgentActivityDisclosure>
  );
}
