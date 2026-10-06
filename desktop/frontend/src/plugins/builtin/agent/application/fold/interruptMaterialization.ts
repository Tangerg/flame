import type { AgentInterrupt } from "@/plugins/sdk";
import type { ContentBlock } from "@/plugins/sdk/types/contentBlock";
import type { AgentSessionView } from "@/plugins/sdk/types/agentSessionView";
import { commandString, editableArgs, mapQuestion } from "./projections";
import { appendToTurn, patchRunBlock } from "./fold";
import type { AgentFoldSource } from "./source";

export function materializeInterrupt(
  state: AgentSessionView,
  interrupt: AgentInterrupt,
  source: AgentFoldSource,
): AgentSessionView {
  if (interrupt.type === "approval") {
    if (
      state.messages.some(
        (message) =>
          message.runId === source.runId &&
          message.blocks.some(
            (block) => block.kind === "approval" && block.itemId === interrupt.itemId,
          ),
      )
    ) {
      return patchRunBlock(
        state,
        source.runId,
        (b) => b.kind === "approval" && b.itemId === interrupt.itemId,
        (b) => ({ ...b, rememberable: interrupt.payload.rememberable ?? false }),
      );
    }
    const tool = interrupt.payload.tool;
    const block: ContentBlock = {
      kind: "approval",
      itemId: interrupt.itemId,
      toolName: tool.name,
      command: commandString(tool),
      reason: interrupt.payload.reason ?? "",
      args: editableArgs(tool),
      rememberable: interrupt.payload.rememberable ?? false,
    };
    return appendToTurn(state, source.runId, interrupt.itemId, block, source.timestamp);
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
