import { type ContentBlock } from "@flame/runtime-client";
import { type AgentInput, type AgentInputPart } from "@/features/agent/domain/input";
export function agentInputToContentBlocks(input: AgentInput): ContentBlock[] {
  return input.parts.map((part): ContentBlock => {
    switch (part.kind) {
      case "text":
        return { type: "text", text: part.text };
      case "image":
        return { type: "image", mime: part.mime, data: part.data };
      case "reference":
        return { type: "reference", label: part.label, text: part.text };
    }
  });
}

export function contentBlocksToAgentInput(blocks: readonly ContentBlock[]): AgentInput {
  return {
    parts: blocks.map((block): AgentInputPart => {
      switch (block.type) {
        case "text":
          return { kind: "text", text: block.text };
        case "image":
          return { kind: "image", mime: block.mime, data: block.data };
        case "reference":
          return { kind: "reference", label: block.label, text: block.text };
      }
    }),
  };
}
