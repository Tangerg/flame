export type ApprovalDecision = "approved" | "declined";

/**
 * "plan" is deliberately NOT among these: planning is a session mode the agent enters with
 * its own tools, not a global stance, so a menu entry would set something the runtime no
 * longer has.
 */
export type ApprovalMode = "safe" | "balanced" | "yolo";
export type RememberScope = "session" | "project" | "global";

/**
 * Which interrupt, in which Run, in which Session. The three travel together everywhere a
 * pending question or approval is staged, answered or looked up — a Run identifies the batch
 * a response joins, and an item identifies the one answer within it.
 */
export interface InterruptRef {
  sessionId: string;
  rootRunId: string;
  itemId: string;
}
