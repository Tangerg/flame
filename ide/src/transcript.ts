import type { Item } from "@flame/runtime-contract/wire";

export function itemText(item: Item): string {
  switch (item.type) {
    case "userMessage":
    case "agentMessage":
      return `${item.type === "userMessage" ? "You" : "Flame"}: ${(item.content ?? []).map((block) => (block.type === "text" ? block.text : "[image]")).join("\n")}`;
    case "reasoning":
      return item.redacted ? "[reasoning redacted]" : (item.text ?? "");
    case "question":
      return item.question.fields.map((field) => field.prompt).join("\n");
    case "toolCall": {
      const lines = [`${item.tool.name} [${item.status}]`];
      if (item.approvalDecision === "deny") lines.push("Approval denied");
      if (item.error)
        lines.push(`Error ${item.error.type}${item.error.detail ? `: ${item.error.detail}` : ""}`);
      lines.push(JSON.stringify(item.tool.result ?? item.tool.arguments, null, 2));
      return lines.join("\n");
    }
    case "compaction":
      return `[context compacted]\n${item.summary}`;
  }
}
