import * as stylex from "@stylexjs/stylex";
import { publishStreamFollow } from "./streamFollow";
import type { BlockCtx } from "@/plugins/builtin/chat/message/public/rendering";
import type { TranscriptRow } from "@/plugins/builtin/agent/public/conversation";
import type { Message } from "@/plugins/sdk/types/agentSessionView";
import { AnimatePresence, motion } from "motion/react";
import { memo, useEffect } from "react";
import { StickToBottom, useStickToBottomContext } from "use-stick-to-bottom";
import { enterUp } from "@/lib/motion";
import { cn } from "@/lib/classNames";
import { dayKey, formatDay } from "@/lib/i18n/relativeTime";
import { useT } from "@/lib/i18n";
import { Divider } from "@/ui";
import { readingColumn as rc } from "./readingColumn";
import {
  useCurrentRootMaterial,
  type CurrentRootMaterial,
} from "@/plugins/builtin/agent/public/run";
import {
  finalAnswerFollows,
  MessageBlock,
  RootRunOutcome,
} from "@/plugins/builtin/chat/message/public/rendering";
import { transcriptTurnContentVisibility } from "./transcriptTurnContentVisibility";
import { space } from "@/styles/tokens.stylex";

const ms = stylex.create({
  /** The transcript's scrollport: it takes the pane and stops its own overscroll from
   *  reaching the window, which on a webview is what makes the whole app rubber-band. */
  viewport: { minHeight: 0, flex: 1, overflowY: "auto", overscrollBehavior: "contain" },
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
      scrollToBottom: () => void ctx.scrollToBottom(),
    });
  });
  return null;
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

// The gap between turns, as steps rather than class names. Same reason as `seamStep`: a
// distance decided here in Tailwind's alphabet stops existing the day that alphabet does.
const TURN_GAP = stylex.create({
  none: {},
  continuation: { marginTop: space.s2 },
  answer: { marginTop: space.s5 },
  turn: { marginTop: space.s6 },
});

interface TurnProps {
  row: TranscriptRow;
  ctx: BlockCtx;
  sessionId: string;
  isLast: boolean;
  isRunning: boolean;
  answerFollows: boolean;
  terminalRun: CurrentRootMaterial | null;
  opensDay: boolean;
  gap: keyof typeof TURN_GAP;
}

const TranscriptTurn = memo(function TranscriptTurn({
  row,
  ctx,
  sessionId,
  isLast,
  isRunning,
  answerFollows,
  terminalRun,
  opensDay,
  gap,
}: TurnProps) {
  return (
    <>
      {opensDay && <DaySeparator createdAt={row.message.createdAt} />}
      <motion.div
        {...enterUp}
        data-turn-id={row.message.id}
        data-turn-role={row.message.role}
        // The class only: `motion.div` animates through `style`, so spreading StyleX's whole
        // result here hands the same attribute two owners.
        className={
          stylex.props(rc.gutter, TURN_GAP[gap], transcriptTurnContentVisibility(isLast)).className
        }
      >
        <MessageBlock
          row={row}
          ctx={ctx}
          sessionId={sessionId}
          isLast={isLast}
          isRunning={isRunning}
          answerFollows={answerFollows}
          terminalFooter={
            terminalRun ? (
              <div {...stylex.props(ms.afterBlock)}>
                <RootRunOutcome material={terminalRun} />
              </div>
            ) : undefined
          }
        />
      </motion.div>
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
      className={cn("panel-scroll", scroller.className)}
      initial="instant"
      resize="instant"
    >
      <StickToBottom.Content
        scrollClassName={cn(
          // Two mechanism keys `globals.css` owns: the scrollbar's look, and the viewport the
          // transcript measures its own scroll against.
          "panel-scroll msg-scroll-viewport",
          stylex.props(ms.viewport).className,
        )}
        className={stylex.props(rc.box, ms.content).className}
      >
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
                answerFollows={finalAnswerFollows(row.message, rows[index + 1]?.message)}
                terminalRun={index === terminalTurnIndex ? currentRoot : null}
                opensDay={dayBreaks[index] ?? false}
                gap={
                  previous === undefined
                    ? "none"
                    : row.message.phase === "finalAnswer" && previous.runId === row.message.runId
                      ? "answer"
                      : previous.role === row.message.role && previous.runId === row.message.runId
                        ? "continuation"
                        : "turn"
                }
              />
            );
          })}
        </AnimatePresence>
        <div aria-hidden {...stylex.props(rc.clearance)} />
      </StickToBottom.Content>
      <ControlsRelay />
    </StickToBottom>
  );
}
