import { type ToolActionSpec } from "@/features/chat/public/tools/types";
import { type ToolCall } from "@/features/agent/public/sessionView";
export interface CopyToolArgsActionOptions {
  title: string;
  copyText: (text: string) => unknown | Promise<unknown>;
}

export function hasCopyableToolArgs(tool: Pick<ToolCall, "args">): boolean {
  return tool.args.trim().length > 0;
}

export function copyToolArgsAction(options: CopyToolArgsActionOptions): ToolActionSpec {
  return {
    id: "copy-args",
    icon: "copy",
    title: options.title,
    order: 0,
    predicate: hasCopyableToolArgs,
    run: async (tool) => {
      await options.copyText(tool.args);
    },
  };
}
