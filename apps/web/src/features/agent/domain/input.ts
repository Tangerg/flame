interface AgentTextInput {
  kind: "text";
  text: string;
}

interface AgentImageInput {
  kind: "image";
  mime: string;
  data: string;
}

interface AgentReferenceInput {
  kind: "reference";
  label: string;
  text: string;
}

export type AgentInputPart = AgentTextInput | AgentImageInput | AgentReferenceInput;

export interface AgentInput {
  parts: AgentInputPart[];
}

export function agentTextInput(text: string): AgentInput {
  return text ? { parts: [{ kind: "text", text }] } : { parts: [] };
}
