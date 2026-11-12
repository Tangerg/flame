import { type Ref } from "react";
import { useStreamFollow } from "@/ui/agent/stream-follow";
import { AgentJumpToBottom } from "@/ui/agent/index";

export function JumpToBottomButton({ dockRef }: { dockRef?: Ref<HTMLDivElement> }) {
  const follow = useStreamFollow();
  const visible = !!follow && !follow.isAtBottom;
  return (
    <AgentJumpToBottom
      visible={visible}
      onClick={() => {
        follow?.contentRef.current?.focus({ preventScroll: true });
        void follow?.scrollToBottom();
      }}
      placement="above-composer"
      dockRef={dockRef}
    />
  );
}
