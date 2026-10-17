import type { InterruptResumeInput } from "@/plugins/builtin/agent/domain/hitl";
import type { RuntimeAgentSessionSnapshot } from "./agentSessionSnapshots";
import { VISUAL_NOW } from "./agentFixtureFacts";

export function commitVisualInterruptResponses(
  snapshot: RuntimeAgentSessionSnapshot,
  responses: InterruptResumeInput[],
): void {
  for (const { itemId, response } of responses) {
    const group = snapshot.pendingInterruptSets.find((pending) =>
      pending.interrupts.some((interrupt) => interrupt.itemId === itemId),
    );
    const interrupt = group?.interrupts.find((candidate) => candidate.itemId === itemId);
    if (!interrupt || !group) throw new Error(`No pending visual interrupt for "${itemId}"`);
    const previous = snapshot.items.find((item) => item.id === itemId);
    if (interrupt.type === "approval" && response.type === "approval") {
      const approved = response.decision === "approve";
      snapshot.items = [
        ...snapshot.items.filter((item) => item.id !== itemId),
        {
          type: "toolCall",
          id: itemId,
          runId: interrupt.runId,
          startedAt: previous?.type === "toolCall" ? previous.startedAt : group.createdAt,
          status: approved ? "running" : "incomplete",
          approvalDecision: approved ? "approve" : "deny",
          tool: {
            ...interrupt.payload.tool,
            arguments: response.editedArgs ?? interrupt.payload.tool.arguments,
          },
          ...(approved
            ? {}
            : {
                finishedAt: new Date(VISUAL_NOW).toISOString(),
                durationMillis:
                  VISUAL_NOW -
                  Date.parse(previous?.type === "toolCall" ? previous.startedAt : group.createdAt),
                error: { type: "denied_by_user" },
              }),
        },
      ];
    } else if (interrupt.type === "question" && response.type === "answer") {
      snapshot.items = [
        ...snapshot.items.filter((item) => item.id !== itemId),
        {
          type: "question",
          id: itemId,
          runId: interrupt.runId,
          createdAt: previous?.type === "question" ? previous.createdAt : group.createdAt,
          status: "completed",
          question: { ...interrupt.payload.question, answers: response.answers },
        },
      ];
    } else throw new Error(`The response does not match visual interrupt "${itemId}"`);
    snapshot.pendingInterruptSets = snapshot.pendingInterruptSets
      .map((pending) => ({
        ...pending,
        interrupts: pending.interrupts.filter((candidate) => candidate.itemId !== itemId),
      }))
      .filter((pending) => pending.interrupts.length > 0);
  }
}
