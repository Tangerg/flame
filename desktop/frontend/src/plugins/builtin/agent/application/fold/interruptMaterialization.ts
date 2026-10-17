import type { AgentInterrupt } from "@/plugins/sdk";
import type { AgentSessionView } from "@/plugins/sdk/types/agentSessionView";
import { mapQuestion } from "./projections";
import { appendToTurn, writeApproval } from "./fold";
import type { AgentFoldSource } from "./source";

export function materializeInterrupt(
  state: AgentSessionView,
  interrupt: AgentInterrupt,
  source: AgentFoldSource,
): AgentSessionView {
  if (interrupt.type === "approval") {
    return writeApproval(
      state,
      { id: interrupt.itemId, runId: source.runId, createdAt: source.timestamp },
      interrupt.payload,
    );
  }
  if (interrupt.type === "question") {
    const hasBlock = state.messages.some(
      (message) =>
        message.runId === source.runId &&
        message.blocks.some(
          (block) => block.kind === "question" && block.itemId === interrupt.itemId,
        ),
    );
    if (hasBlock) return state;
    return appendToTurn(
      state,
      source.runId,
      interrupt.itemId,
      {
        kind: "question",
        status: "complete",
        itemId: interrupt.itemId,
        questions: mapQuestion(interrupt.payload.question),
      },
      source.timestamp,
    );
  }
  return state;
}
