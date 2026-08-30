import type { CSSProperties } from "react";
import { useState } from "react";
import { cn } from "@/lib/classNames";
import { useT } from "@/lib/i18n";
import { formatClock } from "@/lib/i18n/relativeTime";
import { useActiveConversationMessages } from "@/plugins/builtin/agent/public/conversation";
import type { Message } from "@/plugins/builtin/agent/public/viewState";
import { Pressable, RichTooltip } from "@/ui";
import { foldExchanges, scrollToTurn, useTranscriptMap } from "../adapters/transcriptAnchors";

const REACH = 3;

const MAGNIFY = 16;

const FLOOR = 8;

const SHARE = 10;

const TRACK = FLOOR + SHARE + MAGNIFY;

export function TurnRail() {
  const t = useT();
  const messages = useActiveConversationMessages();
  const { visibleTurnId, turns: extents } = useTranscriptMap();
  const [reached, setReached] = useState<number | null>(null);
  const turns = foldExchanges(messages);
  if (turns.length < 2) return null;

  const shareOf = (id: string) => extents.find((extent) => extent.id === id)?.share ?? 0;
  const swell = (index: number) =>
    reached === null ? 0 : Math.max(0, 1 - Math.abs(index - reached) / REACH);

  return (
    <nav
      aria-label={t("narrative.rail.turns")}
      className="flex h-full w-fit flex-col items-start justify-center overflow-hidden py-6 pl-6"
      onPointerLeave={() => setReached(null)}
    >
      {turns.map((turn, index) => {
        const active = turn.id === visibleTurnId;
        const lead = reached === null ? active : reached === index;
        return (
          <RichTooltip
            key={turn.id}
            side="right"
            sideOffset={12}
            className="w-[276px] rounded-[var(--floating-panel-radius)] bg-card p-0"
            trigger={
              <Pressable
                type="button"
                data-chrome-focus=""
                aria-current={active ? "true" : undefined}
                aria-label={turnLabel(t, turn, index, turns.length)}
                onPointerEnter={() => setReached(index)}
                onFocus={() => setReached(index)}
                onBlur={() => setReached(null)}
                onClick={() => scrollToTurn(turn.id)}
                className="flex h-[9px] shrink-0 items-center"
                style={{ width: `${TRACK}px` } as CSSProperties}
              >
                <span
                  className={cn(
                    "h-[2px] rounded-pill transition-[background-color,width] duration-[var(--dur-fast)]",
                    lead ? "bg-fg" : "bg-fg-faint/55",
                  )}
                  style={
                    {
                      width: `${FLOOR + Math.round(shareOf(turn.id) * SHARE + swell(index) * MAGNIFY)}px`,
                    } as CSSProperties
                  }
                />
              </Pressable>
            }
          >
            <TurnPreview turn={turn} answer={answerAfter(messages, turn.id)} />
          </RichTooltip>
        );
      })}
    </nav>
  );
}

function turnLabel(
  t: ReturnType<typeof useT>,
  turn: Message,
  index: number,
  total: number,
): string {
  const stamp = formatClock(turn.createdAt);
  return `${t("role.user")} ${index + 1}/${total}${stamp ? ` · ${stamp}` : ""}`;
}

function answerAfter(messages: Message[], turnId: string): Message | undefined {
  const index = messages.findIndex((message) => message.id === turnId);
  if (index < 0) return undefined;
  return messages.slice(index + 1).find((message) => message.role === "assistant");
}

function proseOf(message: Message | undefined): string {
  if (!message) return "";
  for (const block of message.blocks) {
    if (block.kind !== "text") continue;
    const plain = block.text
      .replace(/```[\s\S]*?```/g, " ")
      .replace(/[#>*_`~-]/g, " ")
      .replace(/\s+/g, " ")
      .trim();
    if (plain) return plain;
  }
  return "";
}

function TurnPreview({ turn, answer }: { turn: Message; answer: Message | undefined }) {
  const t = useT();
  const question = proseOf(turn);
  const reply = proseOf(answer);

  return (
    <div className="flex flex-col gap-1.5 px-3.5 py-3 text-left">
      <span className="line-clamp-1 text-ui-md font-medium leading-snug text-fg">
        {question || t("role.user")}
      </span>
      {reply && <span className="line-clamp-3 text-ui-sm leading-body text-fg-muted">{reply}</span>}
    </div>
  );
}
