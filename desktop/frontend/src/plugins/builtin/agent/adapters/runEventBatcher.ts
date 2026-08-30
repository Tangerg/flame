import type { RunEvent } from "@/rpc";

type ScheduleFrame = (flush: () => void) => number;
type CancelFrame = (handle: number) => void;

/** A hidden WebView may suspend animation frames while Runtime streaming stays
 * active. This is the maximum material kept waiting for one visual frame. */
export const MAXIMUM_RUN_EVENTS_PER_FRAME = 256;

export interface RunEventBatcher {
  enqueue(event: RunEvent): void;
  /** Apply the queued stream tail synchronously. Projection synchronization
   *  calls this before it is allowed to read a newer durable snapshot, so an
   *  animation-frame delay cannot reorder live facts behind that snapshot. */
  flush(): void;
  dispose(): void;
}

interface RunEventBatcherOptions {
  readEpoch: () => bigint;
  /** True only when the whole batch was folded into the current projection. */
  apply: (batch: RunEvent[]) => boolean;
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

    if (!apply(batch)) return;
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
