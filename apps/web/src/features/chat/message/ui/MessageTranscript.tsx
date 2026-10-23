import * as stylex from "@stylexjs/stylex";
import { AnimatePresence } from "motion/react";
import { memo, type ComponentProps, type ReactNode } from "react";
import { type TranscriptRow } from "@/features/agent/public/conversation";
import { type Message } from "@/features/agent/public/sessionView";
import { dayKey, formatDay } from "@/lib/i18n/relativeTime";
import { useT } from "@/lib/i18n/index";
import { Divider } from "@/ui/index";
import { space } from "@flame/ui/tokens.stylex";
import { type BlockCtx } from "@/features/chat/message/ui/blockContext";
import { MessageTurn } from "@/features/chat/message/ui/MessageTurn";
import { readingColumn as rc } from "@/features/chat/message/ui/readingColumn";
import { transcriptTurnContentVisibility } from "@/features/chat/message/ui/transcriptTurnContentVisibility";
import { TurnErrorBoundary } from "@/features/chat/message/ui/TurnErrorBoundary";

const styles = stylex.create({ dayPad: { paddingBlock: space.s1 } });

interface Props {
  rows: readonly TranscriptRow[];
  ctx: BlockCtx;
  sessionId: string;
  isRunning: boolean;
  terminalFooter?: { messageId: string; content: ReactNode };
}

function DaySeparator({ createdAt }: { createdAt?: string }) {
  useT();
  const label = formatDay(createdAt);
  if (!label) return null;
  return (
    <div {...stylex.props(rc.gutter, styles.dayPad)}>
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

const TranscriptTurn = memo(function TranscriptTurn({
  opensDay,
  ...props
}: Omit<ComponentProps<typeof MessageTurn>, "className"> & { opensDay: boolean }) {
  return (
    <>
      {opensDay && <DaySeparator createdAt={props.row.message.createdAt} />}
      <MessageTurn
        {...props}
        className={stylex.props(rc.gutter, transcriptTurnContentVisibility(props.isLast)).className}
      />
    </>
  );
});

export function MessageTranscript({ rows, ctx, sessionId, isRunning, terminalFooter }: Props) {
  const dayBreaks = transcriptDayBreaks(rows);
  return (
    <AnimatePresence initial={false}>
      {rows.map((row, index) => (
        <TurnErrorBoundary key={row.message.id} resetKey={row.message.id}>
          <TranscriptTurn
            row={row}
            ctx={ctx}
            sessionId={sessionId}
            isLast={index === rows.length - 1}
            isRunning={isRunning}
            previous={rows[index - 1]?.message}
            next={rows[index + 1]?.message}
            opensDay={dayBreaks[index] ?? false}
            terminalFooter={
              terminalFooter?.messageId === row.message.id ? terminalFooter.content : undefined
            }
          />
        </TurnErrorBoundary>
      ))}
    </AnimatePresence>
  );
}
