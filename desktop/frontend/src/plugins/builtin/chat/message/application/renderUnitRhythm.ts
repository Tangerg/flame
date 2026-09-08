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

/** How far apart two units sit. Four distances, named by how far and not by how much. */
export type UnitSeam = "tight" | "close" | "apart" | "wide";

/**
 * Keyed on the PAIR, because a seam is a relationship and neither side knows the distance
 * alone. This table is the ONLY owner of the RELATIONSHIP — cards must not set outer margins:
 * adjacent margins collapse, so per-card values made the gap depend on which pair happened
 * to meet.
 *
 * What it no longer owns is the number of pixels. These were Tailwind class names — `"mt-4"`,
 * decided in an application module — which is a layer violation and a silent one: the day the
 * theme stopped generating that utility the transcript's whole rhythm collapsed to zero and
 * nothing said so. The view maps a seam to a step; this table says which seam a pair makes.
 */
const SEAM: Record<UnitVoice, Record<UnitVoice, UnitSeam>> = {
  process: { process: "tight", prose: "wide", panel: "apart" },
  prose: { process: "wide", prose: "close", panel: "apart" },
  panel: { process: "apart", prose: "wide", panel: "close" },
};

export function unitSeam(
  previous: MessageRenderUnit | undefined,
  unit: MessageRenderUnit,
): UnitSeam | undefined {
  if (!previous) return undefined;
  return SEAM[unitVoice(previous)][unitVoice(unit)];
}

// Flat: at the top level there is nothing for a unit to be subordinate to, so a step in from
// the measure only moves it out of the reading column. Kept as a named fact rather than an
// empty string per voice, which is what it was — three keys all answering "none".
