import * as stylex from "@stylexjs/stylex";
import { useState, type ReactNode } from "react";
import { type MessageRenderUnit } from "@/features/agent/public/messagePresentation";
import { summarizeActivity } from "@/features/agent/public/messagePresentation";
import { waveStepCount, waveToolCalls } from "@/features/agent/public/messagePresentation";
import { AgentActivityDisclosure } from "@/ui/agent/index";
import { ToolActivitySummary } from "@/features/chat/public/tools/rendering";
import { ToolGroupRows } from "@/features/chat/public/tools/rendering";
import { useT } from "@/lib/i18n/index";
import { type TurnFacts } from "@/features/agent/public/conversation";
import { unitSeam } from "@/features/chat/message/application/renderUnitRhythm";
import { renderUnitKey } from "@/features/chat/message/application/renderUnitAnchor";
import { seamStep } from "@/features/chat/message/ui/messageStyles";
import { type BlockCtx } from "@/features/chat/message/ui/blockContext";
import { waveGlyph } from "@/features/chat/message/ui/narrativeWaveGlyphs";
interface Props {
  messageId: string;
  units: MessageRenderUnit[];
  facts: TurnFacts;
  ctx: BlockCtx;
  renderUnit: (
    messageId: string,
    unit: MessageRenderUnit,
    facts: TurnFacts,
    ctx: BlockCtx,
  ) => ReactNode;
}

export function NarrativeWave({ messageId, units, facts, ctx, renderUnit }: Props) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const tools = waveToolCalls(units, facts.toolCalls);
  const summary = summarizeActivity(t, tools);
  const steps = t("agent.steps", { count: waveStepCount(units) });

  return (
    <AgentActivityDisclosure
      shell="line"
      contentInset="rows"
      icon={waveGlyph(units, tools)}
      label={<ToolActivitySummary label={summary || steps} tools={tools} />}
      trailing={summary ? steps : undefined}
      open={open}
      onToggle={() => setOpen((value) => !value)}
      stickyHeader
    >
      {units.map((unit, index) => (
        <div
          key={renderUnitKey(messageId, unit)}
          {...stylex.props(seamStep[unitSeam(units[index - 1], unit) ?? "none"])}
        >
          {unit.kind === "toolGroup" ? (
            <ToolGroupRows
              tools={unit.tools}
              expandedIds={ctx.expandedIds}
              onToggleExpand={ctx.onToggleExpand}
            />
          ) : (
            renderUnit(messageId, unit, facts, ctx)
          )}
        </div>
      ))}
    </AgentActivityDisclosure>
  );
}
