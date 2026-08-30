// Takes the `line` shell: a fold over process is itself process, and a card here would
// make the thing being hidden heavier than the answer it sits above.

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
  /** The transcript's own unit dispatcher, injected rather than imported: a fold is
   *  one CASE of that dispatch, so importing it back would close a cycle. Same
   *  arrangement DelegatedNarrative uses for the same reason. */
  renderUnit: (unit: MessageRenderUnit, facts: TurnFacts, ctx: BlockCtx) => ReactNode;
}

export function NarrativeWave({ units, facts, ctx, renderUnit }: Props) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const tools = waveToolCalls(units, facts.toolCalls);
  // A count alone ("6 steps") makes the reader open the row to learn whether anything was
  // changed or run — the one thing a folded account of past work must say while closed.
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
      // A wave holds a whole round of work and is routinely taller than the reading column.
      stickyHeader
    >
      {/* Each member already knows it is superseded — this wave exists BECAUSE an
          answer followed it — so nothing inside springs open when the wave does.
          Same seam owner as the transcript: a fold holds the rows it would otherwise
          have shown inline, so it has to space them the same way. */}
      {units.map((unit, index) => (
        <div key={index} className={unitSeamClass(units[index - 1], unit)}>
          {renderUnit(unit, facts, ctx)}
        </div>
      ))}
    </AgentActivityDisclosure>
  );
}
