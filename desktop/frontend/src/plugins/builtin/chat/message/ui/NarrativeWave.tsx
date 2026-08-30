import { useState, type ReactNode } from "react";
import type { MessageRenderUnit } from "@/plugins/builtin/agent/public/messagePresentation";
import {
  summarizeActivity,
  waveStepCount,
  waveToolCalls,
} from "@/plugins/builtin/agent/public/messagePresentation";
import { AgentActivityDisclosure } from "@/ui/agent";
import { useT } from "@/lib/i18n";
import type { TurnFacts } from "@/plugins/builtin/agent/public/conversation";
import { unitSeamClass } from "../application/renderUnitRhythm";
import type { BlockCtx } from "./blockContext";
import { waveGlyph } from "./narrativeWaveGlyphs";

interface Props {
  units: MessageRenderUnit[];
  facts: TurnFacts;
  ctx: BlockCtx;
  renderUnit: (unit: MessageRenderUnit, facts: TurnFacts, ctx: BlockCtx) => ReactNode;
}

export function NarrativeWave({ units, facts, ctx, renderUnit }: Props) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const tools = waveToolCalls(units, facts.toolCalls);
  const summary = summarizeActivity(t, tools);
  const steps = t("agent.steps", { count: waveStepCount(units) });

  return (
    <AgentActivityDisclosure
      shell="line"
      icon={waveGlyph(units, facts.toolCalls) ?? "sparkle"}
      label={summary || steps}
      trailing={summary ? steps : undefined}
      open={open}
      onToggle={() => setOpen((value) => !value)}
      stickyHeader
    >
      {units.map((unit, index) => (
        <div key={index} className={unitSeamClass(units[index - 1], unit)}>
          {renderUnit(unit, facts, ctx)}
        </div>
      ))}
    </AgentActivityDisclosure>
  );
}
