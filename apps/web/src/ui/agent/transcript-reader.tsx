import {
  createContext,
  useCallback,
  useContext,
  useLayoutEffect,
  useMemo,
  useState,
  type Dispatch,
  type PropsWithChildren,
  type SetStateAction,
} from "react";
import type { ReadingListHandle } from "@flame/ui";

export interface TranscriptReader {
  readonly sessionId: string;
  readonly list: ReadingListHandle;
  readonly atEnd: boolean;
}

const TranscriptReaderContext = createContext<{
  reader: TranscriptReader | null;
  publish: Dispatch<SetStateAction<TranscriptReader | null>>;
  complete: boolean;
  requireComplete(required: boolean): void;
} | null>(null);

export function TranscriptReaderProvider({
  sessionId,
  children,
}: PropsWithChildren<{ sessionId: string }>) {
  const [published, publish] = useState<TranscriptReader | null>(null);
  const [completeSession, setCompleteSession] = useState<string | null>(null);
  const reader = published?.sessionId === sessionId ? published : null;
  const complete = completeSession === sessionId;
  const requireComplete = useCallback(
    (required: boolean) => {
      setCompleteSession((current) => {
        if (required) return sessionId;
        return current === sessionId ? null : current;
      });
    },
    [sessionId],
  );
  const value = useMemo(
    () => ({ reader, publish, complete, requireComplete }),
    [reader, complete, requireComplete],
  );
  return <TranscriptReaderContext value={value}>{children}</TranscriptReaderContext>;
}

export function useTranscriptReader(): TranscriptReader | null {
  return useContext(TranscriptReaderContext)?.reader ?? null;
}

export function usePublishTranscriptReader(reader: TranscriptReader | null) {
  const publish = useContext(TranscriptReaderContext)?.publish;
  useLayoutEffect(() => {
    if (!publish) return;
    publish(reader);
    // A retiring transcript must not clear its replacement's reader.
    return () => publish((current) => (current === reader ? null : current));
  }, [reader, publish]);
}

export function useCompleteTranscriptRequired(): boolean {
  return useContext(TranscriptReaderContext)?.complete ?? false;
}

// Find derives matches from rendered text, so every row must exist while it searches.
export function useRequireCompleteTranscript(required: boolean) {
  const requireComplete = useContext(TranscriptReaderContext)?.requireComplete;
  useLayoutEffect(() => {
    if (!requireComplete || !required) return;
    requireComplete(true);
    return () => requireComplete(false);
  }, [requireComplete, required]);
}
