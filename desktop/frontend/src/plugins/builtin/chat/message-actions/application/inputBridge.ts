import type { AgentInput } from "@/plugins/builtin/agent/public/input";
import type { ComposerDraftInput } from "../../composer/public/draft";
export { composerInputToAgentInput } from "../../composer/public/sendToAgent";

export function agentInputToComposerDraft(input: AgentInput): ComposerDraftInput {
  return {
    text: input.parts
      .filter(
        (part): part is Extract<AgentInput["parts"][number], { kind: "text" }> =>
          part.kind === "text",
      )
      .map((part) => part.text)
      .join("\n\n"),
    images: input.parts
      .filter(
        (part): part is Extract<AgentInput["parts"][number], { kind: "image" }> =>
          part.kind === "image",
      )
      .map((part) => ({ mime: part.mime, data: part.data })),
  };
}
