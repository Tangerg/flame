import type { AgentItem } from "@/plugins/sdk";
import type { BlockStatus, ContentBlock } from "@/plugins/sdk/types/contentBlock";
import type { AgentSessionView, Message, ToolCall } from "@/plugins/sdk/types/agentSessionView";
import {
  argsText,
  contentText,
  mapQuestion,
  mapQuestionAnswers,
  toolFields,
  toolLabel,
  toolLabelKind,
  toolStatus,
  userContentBlocks,
} from "./projections";

function mutateMessage(
  state: AgentSessionView,
  id: string,
  fn: (m: Message) => Message,
): AgentSessionView {
  return { ...state, messages: state.messages.map((m) => (m.id === id ? fn(m) : m)) };
}

function ensureTurn(
  state: AgentSessionView,
  runId: string,
  itemId: string,
  createdAt?: string,
): { state: AgentSessionView; id: string } {
  const assistantTurnMessageId = state.assistantTurnByRunId[runId];
  const open =
    assistantTurnMessageId &&
    state.messages.some((message) => message.id === assistantTurnMessageId)
      ? assistantTurnMessageId
      : null;
  if (open) return { state, id: open };
  const id = `turn:${itemId}`;
  if (state.messages.some((m) => m.id === id)) {
    return {
      state: {
        ...state,
        assistantTurnByRunId: { ...state.assistantTurnByRunId, [runId]: id },
      },
      id,
    };
  }

  const msg: Message = {
    id,
    role: "assistant",
    phase: "commentary",
    createdAt,
    runId,
    blocks: [],
  };
  return {
    state: {
      ...state,
      messages: [...state.messages, msg],
      assistantTurnByRunId: { ...state.assistantTurnByRunId, [runId]: id },
    },
    id,
  };
}

export function appendToTurn(
  state: AgentSessionView,
  runId: string,
  itemId: string,
  block: ContentBlock,
  createdAt?: string,
): AgentSessionView {
  const { state: next, id } = ensureTurn(state, runId, itemId, createdAt);
  return mutateMessage(next, id, (message) => ({
    ...message,
    blocks: [...message.blocks, block],
  }));
}

export function patchRunBlock(
  state: AgentSessionView,
  runId: string,
  match: (block: ContentBlock) => boolean,
  patch: (block: ContentBlock) => ContentBlock,
): AgentSessionView {
  let patched = false;
  const messages = state.messages.map((message) => {
    if (patched || message.runId !== runId || !message.blocks.some(match)) return message;
    patched = true;
    return {
      ...message,
      blocks: message.blocks.map((block) => (match(block) ? patch(block) : block)),
    };
  });
  return patched ? { ...state, messages } : state;
}

function upsertBlock(
  state: AgentSessionView,
  item: { id: string; runId: string; createdAt: string },
  match: (b: ContentBlock) => boolean,
  make: () => ContentBlock,
  patch: (b: ContentBlock) => ContentBlock,
): AgentSessionView {
  if (
    state.messages.some((message) => message.runId === item.runId && message.blocks.some(match))
  ) {
    return patchRunBlock(state, item.runId, match, patch);
  }
  return appendToTurn(state, item.runId, item.id, make(), item.createdAt);
}

export function updateTool(
  state: AgentSessionView,
  runId: string,
  id: string,
  fn: (t: ToolCall) => ToolCall,
): AgentSessionView {
  const existing = state.toolCalls[id];
  if (!existing || existing.runId !== runId) return state;
  return { ...state, toolCalls: { ...state.toolCalls, [id]: fn(existing) } };
}

export function markToolRequiresAction(
  state: AgentSessionView,
  runId: string,
  id: string,
): AgentSessionView {
  return updateTool(state, runId, id, (tool) =>
    tool.status === "requires-action" ? tool : { ...tool, status: "requires-action" },
  );
}

export function settleRunPendingInterrupts(
  state: AgentSessionView,
  runId: string,
): AgentSessionView {
  const owned = state.pendingInterrupts.filter((group) => group.runId === runId);
  if (owned.length === 0) return state;
  const interruptItemIds = new Set(
    owned.flatMap((group) => group.interrupts.map((interrupt) => interrupt.itemId)),
  );
  const actionable = (block: ContentBlock) =>
    (block.kind === "approval" || block.kind === "question") &&
    block.status === "requires-action" &&
    block.itemId !== undefined &&
    interruptItemIds.has(block.itemId);
  const messages = state.messages.map((m) =>
    m.blocks.some(actionable)
      ? {
          ...m,
          blocks: m.blocks.map((b) =>
            actionable(b) ? { ...b, status: "incomplete" as const } : b,
          ),
        }
      : m,
  );
  let toolCalls = state.toolCalls;
  for (const id of interruptItemIds) {
    const tool = toolCalls[id];
    if (!tool || tool.status !== "requires-action") continue;
    toolCalls = { ...toolCalls, [id]: { ...tool, status: "err" } };
  }
  return {
    ...state,
    messages,
    toolCalls,
    pendingInterrupts: state.pendingInterrupts.filter((group) => group.runId !== runId),
  };
}

type ItemOf<T extends AgentItem["type"]> = Extract<AgentItem, { type: T }>;

export function appendUserMessage(
  state: AgentSessionView,
  item: ItemOf<"userMessage">,
): AgentSessionView {
  const durable = state.messages.find((message) => message.id === item.id);
  if (durable) {
    const withOwner =
      durable.runId === item.runId
        ? state
        : {
            ...state,
            messages: state.messages.map((message) =>
              message.id === item.id
                ? {
                    ...message,
                    runId: item.runId,
                    createdAt: item.createdAt,
                    blocks: userContentBlocks(item.content),
                  }
                : message,
            ),
          };
    return closeAssistantTurn(withOwner, item.runId);
  }
  const msg: Message = {
    id: item.id,
    role: "user",
    createdAt: item.createdAt,
    runId: item.runId,
    blocks: userContentBlocks(item.content),
  };
  return closeAssistantTurn({ ...state, messages: [...state.messages, msg] }, item.runId);
}

export function foldText(
  state: AgentSessionView,
  item: ItemOf<"agentMessage">,
  status: BlockStatus,
): AgentSessionView {
  if (item.phase === "finalAnswer") return foldFinalText(state, item, status);
  const text = contentText(item.content);
  return upsertBlock(
    state,
    item,
    (b) => b.kind === "text" && b.itemId === item.id,
    () => ({ kind: "text", itemId: item.id, text, status }),
    (b) => (b.kind === "text" ? { ...b, text: text || b.text, status } : b),
  );
}

function foldFinalText(
  state: AgentSessionView,
  item: ItemOf<"agentMessage">,
  status: BlockStatus,
): AgentSessionView {
  const finalId = `final:${item.id}`;
  const projectedText = contentText(item.content);
  const previous = state.messages
    .flatMap((message) => message.blocks)
    .find(
      (block): block is Extract<ContentBlock, { kind: "text" }> =>
        block.kind === "text" && block.itemId === item.id,
    );
  const block: Extract<ContentBlock, { kind: "text" }> = {
    kind: "text",
    itemId: item.id,
    text: projectedText || previous?.text || "",
    status,
  };

  let foundFinal = false;
  const messages: Message[] = [];
  for (const message of state.messages) {
    if (message.id === finalId) {
      foundFinal = true;
      messages.push({ ...message, phase: "finalAnswer", blocks: [block] });
      continue;
    }
    if (message.runId !== item.runId) {
      messages.push(message);
      continue;
    }
    const blocks = message.blocks.filter(
      (candidate) => !(candidate.kind === "text" && candidate.itemId === item.id),
    );
    if (blocks.length === message.blocks.length) {
      messages.push(message);
    } else if (blocks.length > 0) {
      messages.push({ ...message, phase: "commentary", blocks });
    }
  }

  if (!foundFinal) {
    messages.push({
      id: finalId,
      role: "assistant",
      phase: "finalAnswer",
      createdAt: item.createdAt,
      runId: item.runId,
      blocks: [block],
    });
  }
  return closeAssistantTurn({ ...state, messages }, item.runId);
}

export function foldReasoning(
  state: AgentSessionView,
  item: ItemOf<"reasoning">,
  status: BlockStatus,
): AgentSessionView {
  const text = item.text ?? "";
  return upsertBlock(
    state,
    item,
    (b) => b.kind === "reasoning" && b.reasoningId === item.id,
    () => ({ kind: "reasoning", reasoningId: item.id, text, status }),
    (b) => (b.kind === "reasoning" ? { ...b, text: text || b.text, status } : b),
  );
}

export function foldQuestion(
  state: AgentSessionView,
  item: ItemOf<"question">,
  status: BlockStatus,
): AgentSessionView {
  const questions = mapQuestion(item.question);
  const answers = mapQuestionAnswers(item.question);
  return upsertBlock(
    state,
    item,
    (b) => b.kind === "question" && b.itemId === item.id,
    () => ({
      kind: "question",
      status,
      itemId: item.id,
      questions,
      answered: answers !== undefined,
      answers,
    }),
    (b) =>
      b.kind === "question"
        ? {
            ...b,
            status,
            questions,
            answered: answers !== undefined,
            answers,
          }
        : b,
  );
}

export function foldCompaction(
  state: AgentSessionView,
  item: ItemOf<"compaction">,
): AgentSessionView {
  const block: ContentBlock = {
    kind: "compaction",
    summary: item.summary,
    droppedMessages: item.droppedMessages,
  };
  if (state.messages.some((m) => m.id === item.id)) {
    return mutateMessage(state, item.id, (m) => ({ ...m, blocks: [block] }));
  }
  const msg: Message = {
    id: item.id,
    role: "system",
    createdAt: item.createdAt,
    runId: item.runId,
    blocks: [block],
  };
  return { ...state, messages: [...state.messages, msg] };
}

export function writeToolCall(
  state: AgentSessionView,
  item: ItemOf<"toolCall">,
): { state: AgentSessionView; tool: ToolCall } {
  const withBlock =
    state.toolCalls[item.id] === undefined
      ? appendToTurn(
          state,
          item.runId,
          item.id,
          { kind: "tool", toolCallId: item.id },
          item.startedAt,
        )
      : state;
  const prev = withBlock.toolCalls[item.id];
  const tool: ToolCall = {
    id: item.id,
    runId: item.runId,
    name: item.tool.name,
    fn: toolLabel(item.tool),
    ...(toolLabelKind(item.tool) === "path" ? { fnKind: "path" as const } : {}),
    args:
      item.status === "running" ? (prev?.args ?? "") || argsText(item.tool) : argsText(item.tool),
    status: toolStatus(item),
    result: prev?.result,
    error: item.error ? (item.error.message ?? item.error.code) : undefined,
    durationMillis: item.durationMillis,
    safetyClass: item.safetyClass,
    approvalDecision: item.approvalDecision,
    ...toolFields(item.tool),
  };
  return { state: { ...withBlock, toolCalls: { ...withBlock.toolCalls, [item.id]: tool } }, tool };
}

function closeAssistantTurn(state: AgentSessionView, runId: string): AgentSessionView {
  if (!(runId in state.assistantTurnByRunId)) return state;
  const assistantTurnByRunId = { ...state.assistantTurnByRunId };
  delete assistantTurnByRunId[runId];
  return { ...state, assistantTurnByRunId };
}
