import { useEffect, useState } from "react";

const TURN_ANCHOR_ATTR = "data-turn-id";
const TURN_ROLE_ATTR = "data-turn-role";
const TURN_SELECTOR = `[${TURN_ANCHOR_ATTR}]`;

const READING_LINE = 0.35;

function scroller(): HTMLElement | null {
  return document.querySelector<HTMLElement>(".msg-scroll-viewport");
}

function turnElements(root: HTMLElement): HTMLElement[] {
  return [...root.querySelectorAll<HTMLElement>(TURN_SELECTOR)];
}

interface TurnExtent {
  id: string;
  share: number;
}

export interface AnchoredTurn {
  id: string;
  role: string | null;
  top: number;
}

export function foldExchanges<T extends { id: string; role: string | null }>(
  turns: readonly T[],
): T[] {
  const exchanges: T[] = [];
  for (const turn of turns) {
    if (turn.role === "user" || exchanges.length === 0) exchanges.push(turn);
  }
  return exchanges;
}

export interface TranscriptMap {
  visibleTurnId: string | null;
  turns: TurnExtent[];
}

const EMPTY: TranscriptMap = { visibleTurnId: null, turns: [] };

function sameMap(a: TranscriptMap, b: TranscriptMap): boolean {
  if (a.visibleTurnId !== b.visibleTurnId || a.turns.length !== b.turns.length) return false;
  return a.turns.every((turn, i) => {
    const other = b.turns[i]!;
    return turn.id === other.id && Math.round(turn.share * 20) === Math.round(other.share * 20);
  });
}

export function useTranscriptMap(): TranscriptMap {
  const [map, setMap] = useState<TranscriptMap>(EMPTY);

  useEffect(() => {
    const root = scroller();
    if (!root) return;

    let frame = 0;
    const measure = () => {
      frame = 0;
      const rootTop = root.getBoundingClientRect().top;
      const line = rootTop + root.clientHeight * READING_LINE;
      const contentBottom = rootTop + root.scrollHeight - root.scrollTop;
      const anchored = turnElements(root).map((element) => ({
        id: element.getAttribute(TURN_ANCHOR_ATTR) ?? "",
        role: element.getAttribute(TURN_ROLE_ATTR),
        top: element.getBoundingClientRect().top,
      }));
      const exchanges = foldExchanges(anchored);
      const measured = exchanges.map((exchange, i) => ({
        id: exchange.id,
        top: exchange.top,
        height: (exchanges[i + 1]?.top ?? contentBottom) - exchange.top,
      }));
      const tallest = Math.max(1, ...measured.map((turn) => turn.height));

      let current: string | null = null;
      for (const turn of measured) {
        if (turn.top > line) break;
        current = turn.id;
      }

      const next: TranscriptMap = {
        visibleTurnId: current ?? measured[0]?.id ?? null,
        turns: measured.map((turn) => ({ id: turn.id, share: turn.height / tallest })),
      };
      setMap((previous) => (sameMap(previous, next) ? previous : next));
    };
    const schedule = () => {
      if (frame === 0) frame = requestAnimationFrame(measure);
    };

    measure();
    root.addEventListener("scroll", schedule, { passive: true });
    const observer = new ResizeObserver(schedule);
    observer.observe(root);
    const mutations = new MutationObserver(schedule);
    mutations.observe(root, { childList: true, subtree: true });

    return () => {
      if (frame !== 0) cancelAnimationFrame(frame);
      root.removeEventListener("scroll", schedule);
      observer.disconnect();
      mutations.disconnect();
    };
  }, []);

  return map;
}

function scrollIntoTranscript(target: HTMLElement | null): void {
  const root = scroller();
  if (!root || !target) return;
  const offset = target.getBoundingClientRect().top - root.getBoundingClientRect().top;
  root.scrollTo({ top: root.scrollTop + offset - 24, behavior: "smooth" });
}

export function scrollToTurn(id: string): void {
  scrollToAnchored(TURN_ANCHOR_ATTR, id);
}

function scrollToAnchored(attribute: string, value: string): void {
  const root = scroller();
  scrollIntoTranscript(
    root?.querySelector<HTMLElement>(`[${attribute}="${CSS.escape(value)}"]`) ?? null,
  );
}
