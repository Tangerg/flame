import type { RunEvent } from "@/rpc";

type ScheduleFrame = (flush: () => void) => number;
type CancelFrame = (handle: number) => void;

const MAXIMUM_RUN_EVENTS_PER_FRAME = 256;

export interface RunEventBatcher {
  enqueue(event: RunEvent): void;
  flush(): void;
  dispose(): void;
}

interface RunEventBatcherOptions {
  readEpoch: () => bigint;
  apply: (batch: RunEvent[]) => boolean;
  onFailure: (error: unknown) => void;
  onApplied?: (lastEvent: RunEvent) => void;
  onRunFinished?: () => void;
  scheduleFrame?: ScheduleFrame;
  cancelFrame?: CancelFrame;
  maximumQueuedEvents?: number;
}

export function createRunEventBatcher({
  readEpoch,
  apply,
  onApplied,
  onFailure,
  onRunFinished,
  scheduleFrame = requestAnimationFrame,
  cancelFrame = cancelAnimationFrame,
  maximumQueuedEvents = MAXIMUM_RUN_EVENTS_PER_FRAME,
}: RunEventBatcherOptions): RunEventBatcher {
  if (!Number.isSafeInteger(maximumQueuedEvents) || maximumQueuedEvents <= 0) {
    throw new RangeError("run event frame capacity must be a positive safe integer");
  }
  let queue: RunEvent[] = [];
  let frame: number | null = null;
  let queueEpoch = readEpoch();
  let disposed = false;

  const applyQueued = (): void => {
    if (disposed || queue.length === 0) return;

    const batch = queue;
    queue = [];
    if (readEpoch() !== queueEpoch) {
      queueEpoch = readEpoch();
      return;
    }

    try {
      if (!apply(batch)) {
        disposed = true;
        return;
      }
    } catch (error) {
      disposed = true;
      onFailure(error);
      return;
    }
    onApplied?.(batch[batch.length - 1]!);
    if (batch.some((entry) => entry.event.type === "segment.finished")) onRunFinished?.();
  };

  const flush = (): void => {
    if (frame !== null) {
      cancelFrame(frame);
      frame = null;
    }
    applyQueued();
  };

  return {
    enqueue(event) {
      if (disposed) return;

      const epoch = readEpoch();
      if (epoch !== queueEpoch) {
        queue = [];
        queueEpoch = epoch;
      }
      queue.push(event);
      if (queue.length >= maximumQueuedEvents) {
        flush();
        return;
      }
      if (frame === null)
        frame = scheduleFrame(() => {
          frame = null;
          applyQueued();
        });
    },
    flush,
    dispose() {
      disposed = true;
      queue = [];
      if (frame !== null) cancelFrame(frame);
      frame = null;
    },
  };
}
