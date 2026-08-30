// A PURE selector over item-local material, root attention and position: no store reads and
// no hover state. Hover stays in CSS (`group-hover`) so it costs zero re-renders, and this
// only decides the baseline it layers onto.
//
//   "absent" → not mounted and not in flow (the turn can still grow)
//   "hidden" → mounted and reserved, never shown (a run is streaming)
//   "hover"  → revealed on hover / focus-within
//   "pinned" → always shown

export type MessageActionMaterialization = "active" | "settled";
export type MessageActionsVisibility = "absent" | "hidden" | "hover" | "pinned";

export interface MessageActionsVisibilityInput {
  /** Item-local material is authoritative when root attention crosses a recovery boundary. */
  materialization: MessageActionMaterialization;
  isRunning: boolean;
  isLast: boolean;
}

export function messageActionsVisibility({
  materialization,
  isRunning,
  isLast,
}: MessageActionsVisibilityInput): MessageActionsVisibility {
  if (materialization === "active") return "absent";
  if (isRunning) return "hidden";
  if (isLast) return "pinned";
  return "hover";
}
