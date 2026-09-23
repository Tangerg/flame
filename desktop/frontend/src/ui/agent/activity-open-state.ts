import { useCallback, useState } from "react";

export interface ActivityOpenState {
  open: boolean;
  toggle: () => void;
}

export function useActivityOpenState(live: boolean): ActivityOpenState {
  const [collapsedWhileLive, setCollapsedWhileLive] = useState(false);
  const [openedWhenSettled, setOpenedWhenSettled] = useState(false);

  const toggle = useCallback(() => {
    if (live) setCollapsedWhileLive((value) => !value);
    else setOpenedWhenSettled((value) => !value);
  }, [live]);

  return { open: live ? !collapsedWhileLive : openedWhenSettled, toggle };
}
