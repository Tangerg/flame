import type { MessageRenderUnit } from "@/plugins/builtin/agent/public/messagePresentation";

export type UnitVoice = "process" | "prose" | "panel";

export function unitVoice(unit: MessageRenderUnit): UnitVoice {
  if (unit.kind === "wave" || unit.kind === "toolGroup") return "process";
  switch (unit.block.kind) {
    case "text":
      return "prose";
    case "tool":
    case "reasoning":
    case "compaction":
      return "process";
    case "approval":
    case "question":
    case "image":
      return "panel";
  }
}

export type UnitSeam = "tight" | "close" | "apart" | "wide";

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
