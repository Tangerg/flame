import { publishStreamFollow } from "./streamFollow";
import type { BlockCtx } from "@/plugins/builtin/chat/message/public/rendering";
import type { TranscriptRow } from "@/plugins/builtin/agent/public/conversation";
import type { Message } from "@/plugins/builtin/agent/public/viewState";
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

// The initial landing is `initial="instant"`, a jump: a smooth initial replays a visible
// top→bottom scroll through the whole history on every mount and session switch, which
// reads as auto-scrolling on open and flashes content-visibility gaps on the way past.

interface Props {
  rows: readonly TranscriptRow[];
  ctx: BlockCtx;
  /** Also RE-KEYS scroll position and follow state. */
  sessionId: string;
  controllerRef?: Ref<MessageStreamController>;
}

export interface MessageStreamController {
  settleInitialBottom(): void;
}

// The jump-to-bottom button must be a SIBLING of the scroller to sit over the composer, so
// it cannot read the context itself. Published rather than lifted through setState: the
// context object is rebuilt on every scroll event, so reporting it upward re-renders the
// composer's owner at scroll frequency.
function ControlsRelay() {
  const ctx = useStickToBottomContext();
  // In an effect, not during render: the publish notifies subscribers, which mid-render
  // would update one component while another is rendering. No dep array, so the click
  // handler handed out is never a stale closure.
  useEffect(() => {
    publishStreamFollow({
      atBottom: ctx.isAtBottom,
      scrollToBottom: () => void ctx.scrollToBottom(),
    });
  });
  return null;
}

function DaySeparator({ createdAt }: { createdAt?: string }) {
  // Subscribes to locale changes: the label is formatted, not translated, so nothing else
  // here would re-render on a locale toggle.
  useT();
  const label = formatDay(createdAt);
  if (!label) return null;
  return (
    <div className={cn(READING_GUTTER, "py-1")}>
      <Divider align="start">{label}</Divider>
    </div>
  );
}

// The day rule needs one parse per turn on every render of the list, which is every token
// of a live run — hundreds of date parses per frame for a grouping that cannot change once
// the fold has written the timestamp. A WeakMap keyed on the MESSAGE makes each entry
// collectable the moment the fold replaces it.
const dayKeyByMessage = new WeakMap<Message, string | null>();

function turnDayKey(message: Message): string | null {
  const cached = dayKeyByMessage.get(message);
  if (cached !== undefined) return cached;
  const key = dayKey(message.createdAt);
  dayKeyByMessage.set(message, key);
  return key;
}

function transcriptDayBreaks(rows: readonly TranscriptRow[]): readonly boolean[] {
  // A turn with no timestamp neither opens a day nor breaks the chain: absent means no
  // information, not a different day.
  let previousDay: string | null = null;
  return rows.map((row) => {
    const currentDay = turnDayKey(row.message);
    const opensDay = currentDay !== null && previousDay !== null && currentDay !== previousDay;
    if (currentDay !== null) previousDay = currentDay;
    return opensDay;
  });
}

// Two distances, not one: a flat gap makes a turn's own blocks sit as far apart as two
// separate turns, so nothing groups a thought, its tool call and its answer.
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
  /** Decided by the LIST: it is a relationship between two turns, and no turn can see the
   *  one above it. */
  opensDay: boolean;
  gap: keyof typeof TURN_GAP;
}

// Memoised because the list re-renders on every token of a live run: Motion components are
// NOT memoised, so without this boundary every row's `motion.div` ran a full prop diff and
// visual-element update per delta while the content inside it bailed out. Every prop here
// is a primitive or a reference the transcript projection keeps stable.
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
      {/* NO `layout` prop: Motion re-tweens the block on every text delta, making the
          whole bubble bobble while streaming.

          `content-visibility:auto` skips layout+paint for off-screen messages while
          keeping every node IN the DOM, so ⌘F search, copy-all and stick-to-bottom's
          height all keep working — true virtualization would unmount them. */}
      <motion.div
        {...enterUp}
        data-turn-id={row.message.id}
        data-turn-role={row.message.role}
        // The gutter lives HERE, not on the scroller's content: `content-visibility` brings
        // paint containment, and the turn's last row insets its buttons outward so their
        // glyphs line up with the text. Against a box that hugged the text they come out
        // sliced.
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
  // Height changes are geometry reconciliation, not navigation motion: a smooth resize
  // spring lags streaming and can strand late Shiki / content-visibility growth above the
  // tail once it gives up. The library's user-escape state still decides whether it may
  // move at all.
  const currentRoot = useCurrentRootMaterial();
  const running = currentRoot.running;
  const terminalTurnIndex = currentRoot.terminalTurnIndex(rows);
  const stickContextRef = useRef<StickToBottomContext>(null);

  // The library observes the CONTENT box, but two tail-height sources sit outside it: async
  // Markdown mutates the subtree before its size event, and composer clearance changes
  // padding in the BORDER box. At a compact height the latter can be the only overflow, so
  // a content-box observer never follows it and leaves a blocking HITL action under the
  // composer. Both signals reconcile against the library's own follow bit and target.
  useLayoutEffect(() => {
    const stickContext = stickContextRef.current;
    const viewport = stickContext?.scrollRef.current;
    const content = viewport?.firstElementChild;
    if (!stickContext || !viewport || !content) return;

    const reconcileFollowingTail = () => {
      const current = stickContextRef.current;
      const currentViewport = current?.scrollRef.current;
      // The RAW lock, not the public convenience value: that one stays true inside the
      // library's 70px "near bottom" band, while the lock releases on wheel-up immediately.
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

  // Keeps the scroll library behind a local controller: the parent owns shared
  // composer/transcript geometry but must not know which package implements following.
  // The write lands on the NEXT frame, after the parent's custom-property update has been
  // included in the content height.
  useImperativeHandle(
    controllerRef,
    () => ({
      settleInitialBottom() {
        const stickContext = stickContextRef.current;
        const viewport = stickContext?.scrollRef.current;
        if (!viewport) return;

        // Reading `scrollHeight` commits the parent's just-published composer clearance
        // before the library computes its first target.
        //
        // `ignoreEscapes` covers ONLY this initial reconciliation: content-visibility
        // replaces estimated heights on adjacent frames and those scroll events are not
        // reader intent. The library's ordinary escape owns every later interaction.
        //
        // The library's exact target, not `scrollHeight`: its target sits one pixel above
        // the browser maximum, and overshooting then correcting upward reads as an escape.
        viewport.scrollTop = stickContext.state.calculatedTargetScrollTop;
        void stickContext.scrollToBottom({
          animation: "instant",
          ignoreEscapes: true,
        });
      },
    }),
    [],
  );

  // No empty branch: the only caller mounts this once a transcript exists, while
  // empty home is its own centred layout without a scroller.

  const dayBreaks = transcriptDayBreaks(rows);

  return (
    <StickToBottom
      key={sessionId}
      contextRef={stickContextRef}
      className="panel-scroll msg-scroll"
      initial="instant"
      // Height reconciliation preserves a reading position; it is not a page
      // transition. Making it springy introduces lag and a moving target even
      // for people who never asked the transcript to move.
      resize="instant"
    >
      {/* `msg-scroll-viewport` names the element that actually scrolls. The
          library renders it itself, one level inside the class above, so anything
          outside the transcript that needs the scroll box — the narrative rails —
          would otherwise have to guess at that nesting. */}
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
        {/* Waiting-for-response indicator — the run is live but the assistant
            hasn't opened its turn yet (last message is still the user's). Once
            the assistant message arrives it takes over, so this hides itself. */}
        {running && rows[rows.length - 1]?.message.role === "user" && (
          <div className={cn(READING_GUTTER, "mt-4 flex")}>
            <Loader variant="dots" />
          </div>
        )}
      </StickToBottom.Content>
      <ControlsRelay />
    </StickToBottom>
  );
}
