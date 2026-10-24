import { type Ref } from "react";
import {
  scrollStreamToBottom,
  useStreamAtBottom,
} from "@/features/shell/workbench/panel/streamFollow";
import { AgentJumpToBottom } from "@/ui/agent/index";

export function JumpToBottomButton({ dockRef }: { dockRef?: Ref<HTMLDivElement> }) {
  const visible = !useStreamAtBottom();
  return (
    <AgentJumpToBottom
      visible={visible}
      onClick={scrollStreamToBottom}
      placement="above-composer"
      dockRef={dockRef}
    />
  );
}
