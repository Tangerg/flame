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
import { COMPOSER_CLEARANCE, READING_COLUMN, READING_GUTTER } from "./readingColumn";
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
    <div className={cn(READING_GUTTER, "py-1")}>
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
        className={cn(READING_GUTTER, TURN_GAP[gap], transcriptTurnContentVisibility(isLast))}
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
              <div className="mt-4">
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

  return (
    <StickToBottom
      key={sessionId}
      contextRef={stickContextRef}
      className="panel-scroll msg-scroll"
      initial="instant"
      resize="instant"
    >
      <StickToBottom.Content
        scrollClassName="panel-scroll msg-scroll-viewport"
        className={cn(READING_COLUMN, COMPOSER_CLEARANCE, "relative flex flex-col pt-8")}
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
    <div className={cn(READING_GUTTER, "mt-4 flex")} data-slot="agent-working">
      <Loader
        variant="text-shimmer"
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
