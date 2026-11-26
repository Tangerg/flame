import { createPushPullChannel, type PushPullChannel } from "./channel";
import { observeNotifications } from "./client";
import {
  runEventIsReplayable,
  type RunEvent,
  type RunReplayLimits,
  type RuntimeEvent,
  type SubscribeRunResponse,
} from "@flame/runtime-client/wire";
import { NOTIFICATIONS_RUN_EVENT, NOTIFICATIONS_RUNTIME_EVENT } from "@flame/runtime-client/wire";
import { RpcConnectionError, RpcProtocolError } from "./errors";

interface RunReplayBudget {
  maxEvents: number;
  maxBytes: number;
}

class RunReplayMemory {
  private readonly delivered = new Map<string, number>();
  private readonly encoder = new TextEncoder();
  private readonly maxEvents: number;
  private readonly maxBytes: number;
  private retainedBytes = 0;

  constructor(budget: RunReplayBudget) {
    this.maxEvents = budget.maxEvents;
    this.maxBytes = budget.maxBytes;
    for (const [name, value] of [
      ["event", budget.maxEvents],
      ["byte", budget.maxBytes],
    ] as const) {
      if (!Number.isSafeInteger(value) || value <= 0) {
        throw new RangeError(`run replay ${name} capacity must be a positive safe integer`);
      }
    }
  }

  alreadyDelivered(eventId: string): boolean {
    if (this.delivered.has(eventId)) return true;
    const bytes = this.encoder.encode(eventId).byteLength;
    this.delivered.set(eventId, bytes);
    this.retainedBytes += bytes;
    while (this.delivered.size > this.maxEvents || this.retainedBytes > this.maxBytes) {
      const oldest = this.delivered.keys().next();
      if (oldest.done) break;
      const oldestBytes = this.delivered.get(oldest.value);
      this.delivered.delete(oldest.value);
      this.retainedBytes -= oldestBytes ?? 0;
    }
    return false;
  }
}

export const MAXIMUM_BUFFERED_EPHEMERAL_RUN_EVENTS = 256;

const MAXIMUM_UNNEGOTIATED_REPLAY_IDENTITIES = 2_048;
const MAXIMUM_UNNEGOTIATED_REPLAY_ID_BYTES = 16 * 1024 * 1024;

type RunEventAdmission = "accepted" | "duplicate" | "ephemeralDropped" | "authoritativeOverflow";

class RunEventInbox {
  readonly channel: PushPullChannel<RunEvent>;
  private readonly replayMemory: RunReplayMemory;

  constructor(limits?: RunReplayLimits) {
    const replayBudget: RunReplayBudget = limits ?? {
      maxEvents: MAXIMUM_UNNEGOTIATED_REPLAY_IDENTITIES,
      maxBytes: MAXIMUM_UNNEGOTIATED_REPLAY_ID_BYTES,
    };
    this.replayMemory = new RunReplayMemory(replayBudget);
    const capacity = replayBudget.maxEvents + MAXIMUM_BUFFERED_EPHEMERAL_RUN_EVENTS;
    if (!Number.isSafeInteger(capacity)) {
      throw new RangeError("run event inbox capacity must be a positive safe integer");
    }
    this.channel = createPushPullChannel<RunEvent>({ capacity });
  }

  admit(event: RunEvent): RunEventAdmission {
    const replayable = runEventIsReplayable(event.event.type) === true;
    if (replayable && this.replayMemory.alreadyDelivered(event.eventId)) return "duplicate";
    if (this.channel.tryPush(event)) return "accepted";
    return replayable ? "authoritativeOverflow" : "ephemeralDropped";
  }
}

export function iterableOf<T>(
  channel: PushPullChannel<T>,
  cleanup: () => void,
  signal?: AbortSignal,
): AsyncIterable<T> {
  return {
    [Symbol.asyncIterator]() {
      const inner = channel.iterator();
      const cancel = (): IteratorResult<T> => {
        channel.cancel();
        cleanup();
        return { value: undefined as never, done: true };
      };
      return {
        [Symbol.asyncIterator]() {
          return this;
        },
        next: async (): Promise<IteratorResult<T>> => {
          if (signal?.aborted) return cancel();
          try {
            const result = await inner.next();
            if (result.done) cleanup();
            return result;
          } catch (error) {
            cleanup();
            throw error;
          }
        },
        return: async () => cancel(),
      };
    },
  };
}

function eventStream<T>(channel: PushPullChannel<T>, parent?: AbortSignal) {
  const controller = new AbortController();
  const signal = parent ? AbortSignal.any([parent, controller.signal]) : controller.signal;
  const stop = () => {
    parent?.removeEventListener("abort", dispose);
    controller.abort();
  };
  const dispose = () => {
    channel.cancel();
    stop();
  };
  if (parent?.aborted) dispose();
  else parent?.addEventListener("abort", dispose, { once: true });

  return {
    signal,
    dispose,
    events: iterableOf(channel, stop, parent),
    finish() {
      channel.close();
      stop();
    },
    fail(error: Error) {
      channel.fail(error);
      stop();
    },
  };
}

export interface RunEventStreamOptions {
  signal?: AbortSignal;
  replayLimits?: RunReplayLimits;
}

export type RunStreamTarget = Pick<SubscribeRunResponse, "runId" | "segmentId">;

export function streamRunEvents(options: RunEventStreamOptions = {}) {
  const inbox = new RunEventInbox(options.replayLimits);
  const stream = eventStream(inbox.channel, options.signal);
  return {
    ...stream,
    observe: ({ runId, segmentId }: RunStreamTarget) =>
      observeNotifications(NOTIFICATIONS_RUN_EVENT, {
        next(event, metadata) {
          const matchesRun = event.runId === runId;
          const matchesSegment = event.segmentId === segmentId;
          if (matchesRun !== matchesSegment) {
            stream.fail(
              new RpcProtocolError(
                "Run event",
                [
                  {
                    path: matchesRun ? "$.segmentId" : "$.runId",
                    detail: "must match the acknowledged root Run and Segment",
                  },
                ],
                metadata?.requestId,
              ),
            );
            return;
          }
          const frame = metadata?.eventStream;
          if (
            frame &&
            runEventIsReplayable(event.event.type) === true &&
            frame.cursor !== event.eventId
          ) {
            stream.fail(
              new RpcProtocolError(
                "event-stream frame",
                [{ path: "$.id", detail: "must match the Run event ID" }],
                metadata?.requestId,
              ),
            );
            return;
          }
          const admission = inbox.admit(event);
          if (admission === "authoritativeOverflow") {
            stream.fail(
              new RpcConnectionError(
                "run event consumer exceeded its bounded inbox before an authoritative event",
              ),
            );
          } else if (
            admission === "accepted" &&
            matchesRun &&
            event.event.type === "segment.finished"
          ) {
            stream.finish();
          }
        },
        fail: stream.fail,
        end: (error) =>
          stream.fail(
            error ??
              new RpcConnectionError("run subscription ended before its root segment finished"),
          ),
      }),
  };
}

export const MAXIMUM_BUFFERED_RUNTIME_EVENTS = 64;

export function streamRuntimeEvents(signal?: AbortSignal) {
  const channel = createPushPullChannel<RuntimeEvent>({
    capacity: MAXIMUM_BUFFERED_RUNTIME_EVENTS,
  });
  const stream = eventStream(channel, signal);
  return {
    ...stream,
    observe: () =>
      observeNotifications(NOTIFICATIONS_RUNTIME_EVENT, {
        next({ event }) {
          if (!channel.tryPush(event)) {
            stream.fail(
              new RpcConnectionError("runtime event consumer exceeded its bounded inbox"),
            );
          }
        },
        fail: stream.fail,
        end: (error) =>
          stream.fail(error ?? new RpcConnectionError("runtime subscription ended unexpectedly")),
      }),
  };
}

export interface StreamingResult<R, E> {
  result: R;
  events: AsyncIterable<E>;
}

export async function callOrDispose<R>(
  stream: { dispose: () => void },
  call: () => Promise<R>,
): Promise<R> {
  try {
    return await call();
  } catch (err) {
    stream.dispose();
    throw err;
  }
}
