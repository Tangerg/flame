import { useCallback, useState } from "react";

export interface ActivityOpenState {
  open: boolean;
  toggle: () => void;
}

/**
 * Whether a live activity row is open, remembered as TWO answers rather than one.
 *
 * A row that is still working wants to be open — you asked for it and it is telling you what
 * it is doing. The same row once it has settled wants to be shut, because what it did is now
 * one line of a transcript you are reading past. Those are two different defaults, so a single
 * remembered override cannot serve both: collapsing the thinking while it streamed also
 * decided that the finished thought stays collapsed, and opening a finished one decided that
 * the next live one opens too.
 *
 * Codex's activity disclosure keeps a `collapsedWhileRunning` beside an `expandedWhenSettled`
 * and reads whichever one the status asks for. This is that, with the default for each end
 * stated rather than implied.
 */
export function useActivityOpenState(live: boolean): ActivityOpenState {
  const [collapsedWhileLive, setCollapsedWhileLive] = useState(false);
  const [openedWhenSettled, setOpenedWhenSettled] = useState(false);

  const toggle = useCallback(() => {
    if (live) setCollapsedWhileLive((value) => !value);
    else setOpenedWhenSettled((value) => !value);
  }, [live]);

  return { open: live ? !collapsedWhileLive : openedWhenSettled, toggle };
}
