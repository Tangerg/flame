import { type Ref } from "react";
import { useTranscriptReader } from "@/ui/agent/transcript-reader";
import { AgentJumpToBottom } from "@/ui/agent/index";

export function JumpToBottomButton({ dockRef }: { dockRef?: Ref<HTMLDivElement> }) {
  const reader = useTranscriptReader();
  return (
    <AgentJumpToBottom
      visible={!!reader && !reader.atEnd}
      onClick={() => {
        if (!reader) return;
        reader.list.getScrollableNode()?.focus({ preventScroll: true });
        void reader.list.scrollToEnd({ animated: false });
      }}
      placement="above-composer"
      dockRef={dockRef}
    />
  );
}
