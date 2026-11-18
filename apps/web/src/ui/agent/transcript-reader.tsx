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
import type { ReadingListHandle } from "@flame/ui";

export interface TranscriptReader {
  readonly list: ReadingListHandle;
  readonly atEnd: boolean;
}

const TranscriptReaderContext = createContext<{
  reader: TranscriptReader | null;
  publish: Dispatch<SetStateAction<TranscriptReader | null>>;
  complete: boolean;
  requireComplete: Dispatch<SetStateAction<boolean>>;
} | null>(null);

export function TranscriptReaderProvider({ children }: PropsWithChildren) {
  const [reader, publish] = useState<TranscriptReader | null>(null);
  const [complete, requireComplete] = useState(false);
  const value = useMemo(() => ({ reader, publish, complete, requireComplete }), [reader, complete]);
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
