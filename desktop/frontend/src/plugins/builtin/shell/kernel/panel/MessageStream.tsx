import * as stylex from "@stylexjs/stylex";
import { publishStreamFollow } from "./streamFollow";
import type { BlockCtx } from "@/plugins/builtin/chat/message/public/rendering";
import type { TranscriptRow } from "@/plugins/builtin/agent/public/conversation";
import type { Message } from "@/plugins/sdk/types/agentSessionView";
import { AnimatePresence, motion } from "motion/react";
import { memo, useEffect, useImperativeHandle, useLayoutEffect, useRef, type Ref } from "react";
import {
  StickToBottom,
  useStickToBottomContext,
  type StickToBottomContext,
} from "use-stick-to-bottom";
import { enterUp } from "@/lib/motion";
import { cn } from "@/lib/classNames";
import { dayKey, formatDay } from "@/lib/i18n/relativeTime";
import { useT } from "@/lib/i18n";
import { Divider, Loader } from "@/ui";
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
import { durationText } from "@/plugins/builtin/agent/public/runDigest";
import { useElapsedMillis } from "./useElapsedMillis";
import { space } from "@/styles/tokens.stylex";

const ms = stylex.create({
  /** The transcript's scrollport: it takes the pane and stops its own overscroll from
   *  reaching the window, which on a webview is what makes the whole app rubber-band. */
  viewport: { minHeight: 0, flex: 1, overflowY: "auto", overscrollBehavior: "contain" },
  dayPad: { paddingBlock: space.s1 },
  content: { position: "relative", display: "flex", flexDirection: "column", paddingTop: space.s8 },
  working: { marginTop: space.s4, display: "flex" },
  afterBlock: { marginTop: space.s4 },
  scroller: { minHeight: 0, flex: 1, overflowY: "auto", overscrollBehavior: "contain" },
});

interface Props {
  rows: readonly TranscriptRow[];
  ctx: BlockCtx;
  sessionId: string;
  controllerRef?: Ref<MessageStreamController>;
}

export interface MessageStreamController {
  settleInitialBottom(): void;
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

const TURN_GAP = {
  none: "",
  sameSpeaker: "mt-1",
  newSpeaker: "mt-4",
} as const;

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
        className={cn(
          stylex.props(rc.gutter).className,
          TURN_GAP[gap],
          transcriptTurnContentVisibility(isLast),
        )}
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

export function MessageStream({ rows, ctx, sessionId, controllerRef }: Props) {
  const currentRoot = useCurrentRootMaterial();
  const running = currentRoot.running;
  const terminalTurnIndex = currentRoot.terminalTurnIndex(rows);
  const stickContextRef = useRef<StickToBottomContext>(null);

  useLayoutEffect(() => {
    const stickContext = stickContextRef.current;
    const viewport = stickContext?.scrollRef.current;
    const content = viewport?.firstElementChild;
    if (!stickContext || !viewport || !content) return;

    const reconcileFollowingTail = () => {
      const current = stickContextRef.current;
      const currentViewport = current?.scrollRef.current;
      if (!current?.state.isAtBottom || !currentViewport) return;
      currentViewport.scrollTop = current.state.calculatedTargetScrollTop;
    };
    const mutationObserver = new MutationObserver(reconcileFollowingTail);
    const borderBoxObserver = new ResizeObserver(reconcileFollowingTail);
    mutationObserver.observe(content, { childList: true, characterData: true, subtree: true });
    borderBoxObserver.observe(content, { box: "border-box" });
    return () => {
      mutationObserver.disconnect();
      borderBoxObserver.disconnect();
    };
  }, [sessionId]);

  useImperativeHandle(
    controllerRef,
    () => ({
      settleInitialBottom() {
        const stickContext = stickContextRef.current;
        const viewport = stickContext?.scrollRef.current;
        if (!viewport) return;

        viewport.scrollTop = stickContext.state.calculatedTargetScrollTop;
        void stickContext.scrollToBottom({
          animation: "instant",
          ignoreEscapes: true,
        });
      },
    }),
    [],
  );

  const dayBreaks = transcriptDayBreaks(rows);
  const scroller = stylex.props(ms.scroller);

  return (
    <StickToBottom
      key={sessionId}
      contextRef={stickContextRef}
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
        className={stylex.props(rc.box, rc.clearance, ms.content).className}
      >
        <AnimatePresence initial={false}>
          {rows.map((row, index) => {
            const previousRole = index > 0 ? rows[index - 1]?.message.role : undefined;
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
                  previousRole === undefined
                    ? "none"
                    : previousRole === row.message.role
                      ? "sameSpeaker"
                      : "newSpeaker"
                }
              />
            );
          })}
        </AnimatePresence>
        {running && <WorkingLine startedAt={currentRoot.startedAt} />}
      </StickToBottom.Content>
      <ControlsRelay />
    </StickToBottom>
  );
}

/**
 * That the turn is still going, and how long it has been going for.
 *
 * Wall clock on purpose: this is the wait as lived, including approval pauses that a
 * runtime-measured step duration deliberately excludes.
 */
function WorkingLine({ startedAt }: { startedAt: number | null }) {
  const t = useT();
  const elapsed = useElapsedMillis(startedAt);
  const label = t("agent.working");
  return (
    <div {...stylex.props(rc.gutter, ms.working)} data-slot="agent-working">
      <Loader
        size="sm"
        text={
          startedAt === null
            ? label
            : `${label} · ${durationText(t, startedAt, startedAt + elapsed)}`
        }
      />
    </div>
  );
}
