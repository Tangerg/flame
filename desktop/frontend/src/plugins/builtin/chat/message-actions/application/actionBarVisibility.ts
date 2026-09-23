export type MessageActionMaterialization = "active" | "settled";
export type MessageActionsVisibility = "absent" | "hidden" | "hover" | "pinned";

export interface MessageActionsVisibilityInput {
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
