import { useEffect, useState } from "react";
import type { ReadingListHandle } from "@flame/ui";

const READING_LINE = 0.35;

const TURN_CLEARANCE = 24;

interface TurnExtent {
  id: string;
  share: number;
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

// Unmounted turns have no DOM, so extents come from the list's own layout of every row.
function measure(list: ReadingListHandle, turnIds: readonly string[]): TranscriptMap {
  const state = list.getState();
  const starts = turnIds.flatMap((id) => {
    const index = state.indexByKey(id);
    const top = index === undefined ? undefined : state.positionAtIndex(index);
    return top === undefined ? [] : [{ id, top }];
  });
  const extents = starts.map((turn, i) => ({
    id: turn.id,
    top: turn.top,
    height: (starts[i + 1]?.top ?? state.contentLength) - turn.top,
  }));
  const tallest = Math.max(1, ...extents.map((turn) => turn.height));
  const line = state.scroll + state.scrollLength * READING_LINE;
  let current: string | null = null;
  for (const turn of extents) {
    if (turn.top > line) break;
    current = turn.id;
  }
  return {
    visibleTurnId: current ?? extents[0]?.id ?? null,
    turns: extents.map((turn) => ({ id: turn.id, share: turn.height / tallest })),
  };
}

export function useTranscriptMap(
  list: ReadingListHandle | null,
  turnIds: readonly string[],
): TranscriptMap {
  const [map, setMap] = useState<TranscriptMap>(EMPTY);
  const identity = turnIds.join("\n");

  useEffect(() => {
    if (!list) {
      setMap(EMPTY);
      return;
    }
    const ids = identity ? identity.split("\n") : [];
    let frame = 0;
    const update = () => {
      frame = 0;
      const next = measure(list, ids);
      setMap((previous) => (sameMap(previous, next) ? previous : next));
    };
    const schedule = () => {
      if (frame === 0) frame = requestAnimationFrame(update);
    };

    update();
    const scroller = list.getScrollableNode();
    scroller?.addEventListener("scroll", schedule, { passive: true });
    const state = list.getState();
    const stopPositions = state.listen("lastPositionUpdate", schedule);
    const stopSize = state.listen("totalSize", schedule);

    return () => {
      if (frame !== 0) cancelAnimationFrame(frame);
      scroller?.removeEventListener("scroll", schedule);
      stopPositions();
      stopSize();
    };
  }, [list, identity]);

  return map;
}

export function scrollToTurn(list: ReadingListHandle, id: string): void {
  void list.scrollToKey({ key: id, animated: true, viewOffset: TURN_CLEARANCE });
}
