import * as stylex from "@stylexjs/stylex";
import { publishStreamFollow } from "@/features/shell/workbench/panel/streamFollow";
import { type BlockCtx } from "@/features/chat/public/message/rendering";
import { type TranscriptRow } from "@/features/agent/public/conversation";
import { useEffect, type PropsWithChildren } from "react";
import { StickToBottom, useStickToBottomContext } from "use-stick-to-bottom";
import { cn } from "@/lib/classNames";
import { useContentVisible } from "@/ui/index";
import {
  stopFollowOnDisclosureActivation,
  stopFollowOnReadingFocus,
} from "@/ui/agent/readingFollow";
import { readingColumn as rc } from "@/features/chat/public/message/rendering";
import { composerOverlay } from "@/features/shell/workbench/panel/composerOverlay";
import { useCurrentRootMaterial } from "@/features/agent/public/run";
import { MessageTranscript } from "@/features/chat/public/message/rendering";
import { RootRunOutcome } from "@/features/chat/public/message/rendering";
import { space } from "@flame/ui/tokens.stylex";
const ms = stylex.create({
  viewport: {
    height: "100%",
    width: "100%",
    minHeight: 0,
    flex: 1,
    overflowY: "auto",
    overscrollBehavior: "contain",
    scrollbarGutter: "stable both-edges",
  },
  content: { position: "relative", display: "flex", flexDirection: "column", paddingTop: space.s8 },
  afterBlock: { marginTop: space.s4 },
  scroller: { minHeight: 0, flex: 1, overflowY: "auto", overscrollBehavior: "contain" },
});

interface Props {
  rows: readonly TranscriptRow[];
  ctx: BlockCtx;
  sessionId: string;
}

function ControlsRelay() {
  const ctx = useStickToBottomContext();
  useEffect(() => {
    publishStreamFollow({
      atBottom: ctx.isAtBottom,
      scrollToBottom: () => {
        ctx.contentRef.current?.focus({ preventScroll: true });
        void ctx.scrollToBottom();
      },
    });
  });
  return null;
}

function TranscriptContent({ children }: PropsWithChildren) {
  const follow = useStickToBottomContext();
  const visible = useContentVisible();
  return (
    <div
      ref={visible ? follow.scrollRef : undefined}
      className={cn("msg-scroll-viewport", stylex.props(ms.viewport).className)}
    >
      {/* Native button clicks already include keyboard activation; this reader only observes them. */}
      {/* oxlint-disable-next-line jsx-a11y/click-events-have-key-events, jsx-a11y/no-static-element-interactions */}
      <div
        ref={visible ? follow.contentRef : undefined}
        tabIndex={-1}
        onFocus={(event) => stopFollowOnReadingFocus(event, follow)}
        onClick={(event) => stopFollowOnDisclosureActivation(event, follow)}
        {...stylex.props(rc.box, ms.content)}
      >
        {children}
      </div>
    </div>
  );
}

export function MessageStream({ rows, ctx, sessionId }: Props) {
  const currentRoot = useCurrentRootMaterial();
  const running = currentRoot.running;
  const terminalRow = rows[currentRoot.terminalTurnIndex(rows)];
  const scroller = stylex.props(ms.scroller);

  return (
    <StickToBottom
      key={sessionId}
      {...scroller}
      className={scroller.className}
      initial="instant"
      resize="instant"
    >
      <TranscriptContent>
        <MessageTranscript
          rows={rows}
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
        <div aria-hidden {...stylex.props(composerOverlay.clearance)} />
      </TranscriptContent>
      <ControlsRelay />
    </StickToBottom>
  );
}
