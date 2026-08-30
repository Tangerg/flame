export type ApprovalDecision = "approved" | "declined";

/**
 * "plan" is deliberately NOT among these: planning is a session mode the agent enters with
 * its own tools, not a global stance, so a menu entry would set something the runtime no
 * longer has.
 */
export type ApprovalMode = "safe" | "balanced" | "yolo";
export type RememberScope = "session" | "project" | "global";
