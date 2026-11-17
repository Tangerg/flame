import {
  createContext,
  useContext,
  useLayoutEffect,
  useMemo,
  useState,
  type Dispatch,
  type PropsWithChildren,
  type SetStateAction,
} from "react";
import type { RefObject } from "react";
export interface StreamFollow {
  isAtBottom: boolean;
  scrollToBottom(): void | Promise<void>;
  stopScroll(): void;
  contentRef: RefObject<HTMLElement | null>;
  scrollRef: RefObject<HTMLElement | null>;
}

const StreamFollowContext = createContext<{
  follow: StreamFollow | null;
  publish: Dispatch<SetStateAction<StreamFollow | null>>;
} | null>(null);

export function StreamFollowProvider({ children }: PropsWithChildren) {
  const [follow, publish] = useState<StreamFollow | null>(null);
  const value = useMemo(() => ({ follow, publish }), [follow]);
  return <StreamFollowContext value={value}>{children}</StreamFollowContext>;
}

export function useStreamFollow() {
  return useContext(StreamFollowContext)?.follow ?? null;
}

export function usePublishStreamFollow(follow: StreamFollow | null) {
  const publish = useContext(StreamFollowContext)?.publish;
  useLayoutEffect(() => {
    if (!publish) return;
    publish(follow);
    // A retiring stream must not clear its replacement's controls.
    return () => publish((current) => (current === follow ? null : current));
  }, [follow, publish]);
}
