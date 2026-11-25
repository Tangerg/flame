import * as stylex from "@stylexjs/stylex";
import { type ComponentPropsWithoutRef, useState } from "react";
import { type ToolCall } from "@/features/agent/public/sessionView";
import { toolGroupIconFor } from "@/features/agent/public/toolIcon";
import { toolActivityStatus } from "@/features/agent/public/messagePresentation";
import { AgentActivityDisclosure } from "@/ui/agent/index";
import { useT } from "@/lib/i18n/index";
import {
  toolGroupModel,
  type ToolGroupPinnedState,
} from "@/features/chat/tools/application/toolGroupModel";
import { ToolGroupMember } from "@/features/chat/tools/ui/ToolGroupMember";
import { ToolActivityStatus } from "@/features/chat/tools/ui/ToolActivityStatus";
import { space } from "@flame/ui/tokens.stylex";
interface ToolGroupRowsProps {
  tools: ToolCall[];
  expandedIds: Set<string>;
  onExpandedChange: (id: string, expanded: boolean) => void;
}

interface Props extends ToolGroupRowsProps {
  superseded?: boolean;
}

const tg = stylex.create({
  members: { display: "flex", flexDirection: "column", gap: space.s1 },
});

export function ToolGroup({ tools, expandedIds, onExpandedChange, superseded }: Props) {
  const [pinned, setPinned] = useState<ToolGroupPinnedState>(null);
  const t = useT();
  const model = toolGroupModel(t, tools, pinned, superseded);
  const status = toolActivityStatus(tools);

  return (
    <AgentActivityDisclosure
      icon={toolGroupIconFor(tools)}
      shell="line"
      contentInset="rows"
      label={model.summary}
      // Open rows show their own running, failed and denied states; the count speaks for folded ones.
      detail={!model.expanded && status ? <ToolActivityStatus status={status} /> : undefined}
      detailPlacement="below"
      open={model.expanded}
      onOpenChange={setPinned}
    >
      {/* Focus inside the rows is explicit engagement: pinning keeps settling
          calls from collapsing the body out from under the user's focus. */}
      <ToolGroupRows
        tools={tools}
        expandedIds={expandedIds}
        onExpandedChange={onExpandedChange}
        onFocusCapture={() => setPinned(model.expanded)}
      />
    </AgentActivityDisclosure>
  );
}

export function ToolGroupRows({
  tools,
  expandedIds,
  onExpandedChange,
  onFocusCapture,
}: ToolGroupRowsProps & Pick<ComponentPropsWithoutRef<"div">, "onFocusCapture">) {
  return (
    <div onFocusCapture={onFocusCapture} {...stylex.props(tg.members)}>
      {tools.map((tool) => (
        <ToolGroupMember
          key={tool.id}
          tool={tool}
          expanded={expandedIds.has(tool.id)}
          onExpandedChange={(expanded) => onExpandedChange(tool.id, expanded)}
        />
      ))}
    </div>
  );
}
