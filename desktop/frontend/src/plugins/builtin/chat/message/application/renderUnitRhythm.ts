import type { MessageRenderUnit } from "@/plugins/builtin/agent/public/messagePresentation";

/** `process` is what the turn DID, `prose` is the answer, `panel` stands on its own and
 *  usually wants something from the reader. */
export type UnitVoice = "process" | "prose" | "panel";

export function unitVoice(unit: MessageRenderUnit): UnitVoice {
  if (unit.kind === "wave" || unit.kind === "toolGroup") return "process";
  switch (unit.block.kind) {
    case "text":
      return "prose";
    case "tool":
    case "reasoning":
      return "process";
    default:
      return "panel";
  }
}

/**
 * Keyed on the PAIR, because a seam is a relationship and neither side knows the distance
 * alone. This table is the ONLY owner of the distance — cards must not set outer margins:
 * adjacent margins collapse, so per-card values made the gap depend on which pair happened
 * to meet.
 */
const SEAM: Record<UnitVoice, Record<UnitVoice, string>> = {
  process: { process: "mt-1.5", prose: "mt-5", panel: "mt-4" },
  prose: { process: "mt-5", prose: "mt-3", panel: "mt-4" },
  panel: { process: "mt-4", prose: "mt-5", panel: "mt-3" },
};

export function unitSeamClass(
  previous: MessageRenderUnit | undefined,
  unit: MessageRenderUnit,
): string {
  if (!previous) return "";
  return SEAM[unitVoice(previous)][unitVoice(unit)];
}

// Flat: at the top level there is nothing for a unit to be subordinate to, so a step in from
// the measure only moves it out of the reading column.
const INDENT: Record<UnitVoice, string> = { process: "", prose: "", panel: "" };

export function unitIndentClass(unit: MessageRenderUnit): string {
  return INDENT[unitVoice(unit)];
}
