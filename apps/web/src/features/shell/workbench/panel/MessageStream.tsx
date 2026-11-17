import * as stylex from "@stylexjs/stylex";
import { usePublishStreamFollow } from "@/ui/agent/stream-follow";
import { type BlockCtx } from "@/features/chat/public/message/rendering";
import { type TranscriptRow } from "@/features/agent/public/conversation";
import { useMemo, useState } from "react";
import type { ReadingListHandle } from "@flame/ui";
import { useCurrentRootMaterial } from "@/features/agent/public/run";
import { MessageTranscript } from "@/features/chat/public/message/rendering";
import { RootRunOutcome } from "@/features/chat/public/message/rendering";
import { space } from "@flame/ui/tokens.stylex";
const ms = stylex.create({
  afterBlock: { marginTop: space.s4 },
});

interface Props {
  rows: readonly TranscriptRow[];
  ctx: BlockCtx;
  sessionId: string;
}

export function MessageStream({ rows, ctx, sessionId }: Props) {
  const currentRoot = useCurrentRootMaterial();
  const running = currentRoot.running;
  const terminalRow = rows[currentRoot.terminalTurnIndex(rows)];
  const [state, setState] = useState<{ list: ReadingListHandle; atEnd: boolean } | null>(null);
  const follow = useMemo(
    () =>
      state
        ? {
            isAtBottom: state.atEnd,
            scrollToBottom: () => state.list.scrollToEnd({ animated: false }),
            stopScroll: () => state.list.cancelScroll(),
            contentRef: { current: state.list.getScrollableNode() },
            scrollRef: { current: state.list.getScrollableNode() },
          }
        : null,
    [state],
  );
  usePublishStreamFollow(follow);

  return (
    <MessageTranscript
      className="msg-scroll-viewport"
      key={sessionId}
      rows={rows}
      onFollowChange={(list, atEnd) =>
        setState((current) =>
          current?.list === list && current.atEnd === atEnd ? current : { list, atEnd },
        )
      }
      ctx={ctx}
      sessionId={sessionId}
      isRunning={running}
      terminalFooter={
        terminalRow
          ? {
              messageId: terminalRow.message.id,
              content: (
                <div {...stylex.props(ms.afterBlock)}>
                  <RootRunOutcome material={currentRoot} />
                </div>
              ),
            }
          : undefined
      }
    />
  );
}
