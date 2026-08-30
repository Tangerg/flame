import { useEffect, useState } from "react";

// Both written by the transcript, so the rail can find exchange boundaries from the DOM
// without knowing the conversation model.
export const TURN_ANCHOR_ATTR = "data-turn-id";
const TURN_ROLE_ATTR = "data-turn-role";
const TURN_SELECTOR = `[${TURN_ANCHOR_ATTR}]`;

/** Fraction of the scroller's height at which a turn becomes "the one being read". */
const READING_LINE = 0.35;

// Named by the transcript itself rather than derived from the scroll library's DOM shape.
function scroller(): HTMLElement | null {
  return document.querySelector<HTMLElement>(".msg-scroll-viewport");
}

function turnElements(root: HTMLElement): HTMLElement[] {
  return [...root.querySelectorAll<HTMLElement>(TURN_SELECTOR)];
}

/** `share` is one EXCHANGE's extent as a fraction of the longest one. */
export interface TurnExtent {
  id: string;
  share: number;
}

export interface AnchoredTurn {
  id: string;
  role: string | null;
  top: number;
}

/**
 * Folds anchored turns into exchanges — a question plus everything answering it; a
 * transcript not starting with a user turn still gets a first exchange so nothing is
 * unattributed. Generic over `{id, role}` so the rail folds its MESSAGES through this same
 * function: two callers each filtering for `role === "user"` can disagree about where an
 * exchange begins, and did.
 */
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
    // Quantised: a streaming answer grows a pixel a frame, and repainting the rail sixty
    // times a second moves a tick by nothing a reader can see.
    return turn.id === other.id && Math.round(turn.share * 20) === Math.round(other.share * 20);
  });
}

/**
 * Measured from the DOM rather than tracked in the store: both questions are geometric, and
 * a React-side answer would either re-render the whole list to compute it or go stale the
 * moment a block above grew.
 *
 * ONE hook for both facts — two would install two scroll listeners and force layout twice a
 * frame. rAF coalescing keeps it to one measurement per frame.
 */
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
      // Must be in the same VIEWPORT coordinates as the `getBoundingClientRect().top`
      // reads below. Mixing in content space makes the last exchange measure thousands of
      // pixels, win `tallest`, and round every other mark's share to nothing.
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
        // The first exchange owns the space above the reading line, so a transcript
        // scrolled to the top still highlights something.
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
    // Turn count and turn heights both change while a run streams, and neither fires a
    // scroll event.
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
