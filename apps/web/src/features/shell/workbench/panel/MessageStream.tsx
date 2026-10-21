import * as stylex from "@stylexjs/stylex";
import { AnimatePresence } from "motion/react";
import { publishStreamFollow } from "@/features/shell/workbench/panel/streamFollow";
import { type BlockCtx } from "@/features/chat/public/message/rendering";
import { type TranscriptRow } from "@/features/agent/public/conversation";
import { type Message } from "@/features/agent/public/sessionView";
import { memo, useEffect, type PropsWithChildren } from "react";
import { StickToBottom, useStickToBottomContext } from "use-stick-to-bottom";
import { cn } from "@/lib/classNames";
import { dayKey, formatDay } from "@/lib/i18n/relativeTime";
import { useT } from "@/lib/i18n/index";
import { Divider } from "@/ui/index";
import { useContentVisible } from "@/ui/index";
import { stopFollowOnReadingFocus } from "@/ui/agent/readingFollow";
import { readingColumn as rc } from "@/features/chat/public/message/rendering";
import { composerOverlay } from "@/features/shell/workbench/panel/composerOverlay";
import { useCurrentRootMaterial, type CurrentRootMaterial } from "@/features/agent/public/run";
import { MessageTurn } from "@/features/chat/public/message/rendering";
import { RootRunOutcome } from "@/features/chat/public/message/rendering";
import { transcriptTurnContentVisibility } from "@/features/shell/workbench/panel/transcriptTurnContentVisibility";
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
  dayPad: { paddingBlock: space.s1 },
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
      <div
        ref={visible ? follow.contentRef : undefined}
        tabIndex={-1}
        onFocus={(event) => stopFollowOnReadingFocus(event, follow)}
        {...stylex.props(rc.box, ms.content)}
      >
        {children}
      </div>
    </div>
  );
}

function DaySeparator({ createdAt }: { createdAt?: string }) {
  useT();
  const label = formatDay(createdAt);
  if (!label) return null;
  return (
    <div {...stylex.props(rc.gutter, ms.dayPad)}>
      <Divider align="start">{label}</Divider>
    </div>
  );
}

const dayKeyByMessage = new WeakMap<Message, string | null>();

function turnDayKey(message: Message): string | null {
  const cached = dayKeyByMessage.get(message);
  if (cached !== undefined) return cached;
  const key = dayKey(message.createdAt);
  dayKeyByMessage.set(message, key);
  return key;
}

function transcriptDayBreaks(rows: readonly TranscriptRow[]): readonly boolean[] {
  let previousDay: string | null = null;
  return rows.map((row) => {
    const currentDay = turnDayKey(row.message);
    const opensDay = currentDay !== null && previousDay !== null && currentDay !== previousDay;
    if (currentDay !== null) previousDay = currentDay;
    return opensDay;
  });
}

interface TurnProps {
  row: TranscriptRow;
  ctx: BlockCtx;
  sessionId: string;
  isLast: boolean;
  isRunning: boolean;
  next?: Message;
  terminalRun: CurrentRootMaterial | null;
  opensDay: boolean;
  previous?: Message;
}

const TranscriptTurn = memo(function TranscriptTurn({
  row,
  ctx,
  sessionId,
  isLast,
  isRunning,
  next,
  terminalRun,
  opensDay,
  previous,
}: TurnProps) {
  return (
    <>
      {opensDay && <DaySeparator createdAt={row.message.createdAt} />}
      <MessageTurn
        row={row}
        ctx={ctx}
        sessionId={sessionId}
        isLast={isLast}
        isRunning={isRunning}
        previous={previous}
        next={next}
        className={stylex.props(rc.gutter, transcriptTurnContentVisibility(isLast)).className}
        terminalFooter={
          terminalRun ? (
            <div {...stylex.props(ms.afterBlock)}>
              <RootRunOutcome material={terminalRun} />
            </div>
          ) : undefined
        }
      />
    </>
  );
});

export function MessageStream({ rows, ctx, sessionId }: Props) {
  const currentRoot = useCurrentRootMaterial();
  const running = currentRoot.running;
  const terminalTurnIndex = currentRoot.terminalTurnIndex(rows);
  const dayBreaks = transcriptDayBreaks(rows);
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
        <AnimatePresence initial={false}>
          {rows.map((row, index) => {
            const previous = rows[index - 1]?.message;
            return (
              <TranscriptTurn
                key={row.message.id}
                row={row}
                ctx={ctx}
                sessionId={sessionId}
                isLast={index === rows.length - 1}
                isRunning={running}
                next={rows[index + 1]?.message}
                terminalRun={index === terminalTurnIndex ? currentRoot : null}
                opensDay={dayBreaks[index] ?? false}
                previous={previous}
              />
            );
          })}
        </AnimatePresence>
        <div aria-hidden {...stylex.props(composerOverlay.clearance)} />
      </TranscriptContent>
      <ControlsRelay />
    </StickToBottom>
  );
}
