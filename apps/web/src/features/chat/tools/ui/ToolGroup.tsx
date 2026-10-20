import * as stylex from "@stylexjs/stylex";
import { useState } from "react";
import { type ToolCall } from "@/features/agent/public/sessionView";
import { toolGroupIconFor } from "@/features/agent/public/toolIcon";
import { AgentActivityDisclosure } from "@/ui/agent/index";
import { useT } from "@/lib/i18n/index";
import {
  toolGroupModel,
  type ToolGroupPinnedState,
} from "@/features/chat/tools/application/toolGroupModel";
import { ToolGroupMember } from "@/features/chat/tools/ui/ToolGroupMember";
import { ToolActivitySummary } from "@/features/chat/tools/ui/ToolActivitySummary";
import { space, type as typeStep } from "@flame/ui/tokens.stylex";
import { vocab } from "@flame/ui";
interface ToolGroupRowsProps {
  tools: ToolCall[];
  expandedIds: Set<string>;
  onToggleExpand: (id: string) => void;
}

interface Props extends ToolGroupRowsProps {
  superseded?: boolean;
}

const tg = stylex.create({
  members: { display: "flex", flexDirection: "column", gap: space.s1 },
});

export function ToolGroup({ tools, expandedIds, onToggleExpand, superseded }: Props) {
  const [pinned, setPinned] = useState<ToolGroupPinnedState>(null);
  const t = useT();
  const model = toolGroupModel(t, tools, pinned, superseded);

  return (
    <AgentActivityDisclosure
      icon={toolGroupIconFor(tools)}
      shell="line"
      contentInset="rows"
      label={<ToolActivitySummary label={model.summary} tools={tools} />}
      trailing={
        <span {...stylex.props(vocab.faint, typeStep.uiXs)}>
          {t("tools.group.calls", { count: model.count })}
        </span>
      }
      open={model.expanded}
      onToggle={() => setPinned(model.nextPinned)}
      stickyHeader
    >
      <ToolGroupRows tools={tools} expandedIds={expandedIds} onToggleExpand={onToggleExpand} />
    </AgentActivityDisclosure>
  );
}

export function ToolGroupRows({ tools, expandedIds, onToggleExpand }: ToolGroupRowsProps) {
  return (
    <div {...stylex.props(tg.members)}>
      {tools.map((tool) => (
        <ToolGroupMember
          key={tool.id}
          tool={tool}
          expanded={expandedIds.has(tool.id)}
          onToggleExpand={() => onToggleExpand(tool.id)}
        />
      ))}
    </div>
  );
}
