import * as stylex from "@stylexjs/stylex";
import type { CSSProperties } from "react";
import { useState } from "react";
import { cn } from "@/lib/classNames";
import { useT } from "@/lib/i18n";
import { formatClock } from "@/lib/i18n/relativeTime";
import { useActiveConversationMessages } from "@/plugins/builtin/agent/public/conversation";
import type { Message } from "@/plugins/sdk/types/agentSessionView";
import { Pressable, RichTooltip } from "@/ui";
import { foldExchanges, scrollToTurn, useTranscriptMap } from "../adapters/transcriptAnchors";
import { space, surface, type as typeStep } from "@/styles/tokens.stylex";
import { chatStyles as ct } from "../../chatStyles";

const tr = stylex.create({
  // The rail hangs beside the transcript and is only as wide as its ticks.
  rail: {
    display: "flex",
    height: "100%",
    width: "fit-content",
    flexDirection: "column",
    alignItems: "flex-start",
    justifyContent: "center",
    overflow: "hidden",
    paddingBlock: space.s6,
    paddingLeft: space.s6,
  },
  card: {
    width: "276px",
    borderRadius: "var(--floating-panel-radius)",
    backgroundColor: surface.card,
    padding: 0,
  },
  // A 9px box for a 1px tick: the height is the rail's rhythm, not the mark's.
  tick: { display: "flex", height: "9px", flexShrink: 0, alignItems: "center" },
  preview: {
    display: "flex",
    flexDirection: "column",
    gap: space.s1_5,
    paddingInline: space.s3_5,
    paddingBlock: space.s3,
    textAlign: "left",
  },
});

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
      {...stylex.props(tr.rail)}
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
            className={stylex.props(tr.card).className}
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
                {...stylex.props(tr.tick)}
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
    <div {...stylex.props(tr.preview)}>
      <span {...stylex.props(ct.clampOne, ct.medium, ct.snugLeading, ct.ink, typeStep.uiMd)}>
        {question || t("role.user")}
      </span>
      {reply && (
        <span {...stylex.props(ct.clampThree, ct.bodyLeading, ct.muted, typeStep.uiSm)}>
          {reply}
        </span>
      )}
    </div>
  );
}
