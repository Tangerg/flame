import { type Message } from "@/features/agent/public/sessionView";
import { flattenMarkdown, flattenText } from "@/features/agent/public/messageContent";
export interface MessageCopyPayloads {
  markdown: string;
  plain: string;
  canCopy: boolean;
}

export function messageCopyPayloads(msg: Message): MessageCopyPayloads {
  const markdown = flattenMarkdown(msg.blocks);
  const plain = flattenText(msg.blocks);
  return {
    markdown,
    plain,
    canCopy: Boolean(markdown || plain),
  };
}
