import * as stylex from "@stylexjs/stylex";
import { useState } from "react";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { toolIconFor } from "@/plugins/builtin/chat/tools/public/toolIcon";
import { AgentActivityDisclosure } from "@/ui/agent";
import { useT } from "@/lib/i18n";
import { toolGroupModel, type ToolGroupPinnedState } from "../application/toolGroupModel";
import { ToolGroupMember } from "./ToolGroupMember";
import { space, type as typeStep } from "@/styles/tokens.stylex";
import { chatStyles as ct } from "../../chatStyles";

interface Props {
  tools: ToolCall[];
  onSelectTool: (id: string) => void;
  expandedIds: Set<string>;
  onToggleExpand: (id: string) => void;
  superseded?: boolean;
}

const tg = stylex.create({
  members: { display: "flex", flexDirection: "column", gap: space.s1 },
});

export function ToolGroup({ tools, onSelectTool, expandedIds, onToggleExpand, superseded }: Props) {
  const [pinned, setPinned] = useState<ToolGroupPinnedState>(null);
  const t = useT();
  const model = toolGroupModel(t, tools, pinned, superseded);

  return (
    <AgentActivityDisclosure
      icon={toolIconFor(model.dominantTool)}
      shell="line"
      contentClassName="py-1.5"
      label={model.summary}
      trailing={
        <span {...stylex.props(ct.mono, ct.medium, ct.muted, typeStep.uiXs)}>
          {t("tools.group.calls", { count: model.count })}
        </span>
      }
      open={model.expanded}
      onToggle={() => setPinned(model.nextPinned)}
      stickyHeader
    >
      <div {...stylex.props(tg.members)}>
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
