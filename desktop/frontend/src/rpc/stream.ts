// Server-notification stream → typed AsyncIterable bridge (API.md §5 / §10,
// TRANSPORT.md §7-§9).
//
// v2 collapses run streaming onto ONE notification method:
// `notifications.run.event`, params = RunEvent. There is no separate
// "run closed" method — the terminal signal is a `segment.finished`
// StreamEvent for the ROOT SEGMENT, delivered inside the same stream.
//
// A single response stream is rooted on ONE segment (the segment `runs.start` /
// `runs.resume` / `runs.subscribe` opened). The transport stamps every message
// with the request id of the HTTP response that carried it, so that response is
// the one membership authority for the whole run tree. Reconstructing membership
// from earlier `segment.started` events fails when a reattach begins after those
// events. The stream ends only when the ROOT SEGMENT's `segment.finished` arrives.

import { createPushPullChannel, type PushPullChannel } from "./channel";
import type { RpcClient } from "./client";
import type { RpcId } from "./types";
import {
  runEventIsReplayable,
  type RunEvent,
  type RunReplayLimits,
  type RuntimeEvent,
} from "@flame/runtime-contract/wire";
import { RUNTIME_SUBSCRIBE_METHOD } from "./transport";
import { NOTIFICATIONS_RUN_EVENT, NOTIFICATIONS_RUNTIME_EVENT } from "@flame/runtime-contract/wire";
import { RpcConnectionError } from "./errors";

export const RUN_EVENT_METHOD = NOTIFICATIONS_RUN_EVENT;
export const RUNTIME_EVENT_METHOD = NOTIFICATIONS_RUNTIME_EVENT;

// ---------------------------------------------------------------------------
// Bound run-response projection
// ---------------------------------------------------------------------------

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

  /** Remember replay identities within the negotiated or local safety window. */
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

/** Live previews have no replay identity and may arrive indefinitely. This is
 * the Desktop SDK's own short-burst allowance above the Runtime's advertised
 * authoritative replay count; saturation drops only previews. */
export const MAXIMUM_BUFFERED_EPHEMERAL_RUN_EVENTS = 256;

/** Low-level SDK consumers may intentionally omit discovery. That cannot turn
 * absence of a negotiated replay promise into infinite client retention: these
 * are local safety envelopes, and overflow remains observable/recoverable. */
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

class BoundRunResponse {
  constructor(private readonly rootSegmentId: string) {}

  /** True once the ROOT SEGMENT has finished — ends the stream. A subagent's
   *  segment.finished carries a different segmentId, so it never closes the tree. */
  isRootFinish(ev: RunEvent): boolean {
    return ev.segmentId === this.rootSegmentId && ev.event.type === "segment.finished";
  }
}

// ---------------------------------------------------------------------------
// Channel → AsyncIterable plumbing (shared by every stream below)
// ---------------------------------------------------------------------------

/** Wrap a push-pull channel as a self-cleaning AsyncIterable: `cleanup` runs
 *  once when the iterator drains (done) or the consumer breaks early. */
function iterableOf<T>(channel: PushPullChannel<T>, cleanup: () => void): AsyncIterable<T> {
  return {
    [Symbol.asyncIterator]() {
      const inner = channel.iterator();
      return {
        [Symbol.asyncIterator]() {
          return this;
        },
        next: async (): Promise<IteratorResult<T>> => {
          try {
            const result = await inner.next();
            if (result.done) cleanup();
            return result;
          } catch (error) {
            cleanup();
            throw error;
          }
        },
        return: async (): Promise<IteratorResult<T>> => {
          channel.close();
          cleanup();
          return { value: undefined as never, done: true };
        },
      };
    },
  };
}

interface StreamLifecycle {
  /** Release transport registrations without discarding buffered channel values. */
  cleanup(): void;
  /** Source-side successful termination. */
  close(): void;
  /** Source-side failed termination. */
  fail(error: unknown): void;
  /** Attach registrations after they have been created. */
  bind(unsub: () => void): void;
}

/** Own a channel's transport registrations and cancellation signal.
 *
 * Source termination must release registrations immediately, even when no
 * consumer ever asks the AsyncIterator for `done`. Binding is deliberately
 * deferred until both registrations exist; if an already-aborted signal or a
 * synchronous source callback terminates first, cleanup is remembered and
 * performed as soon as the registrations are attached. */
function createStreamLifecycle<T>(
  channel: PushPullChannel<T>,
  lifetime: StreamLifetime,
): StreamLifecycle {
  const signal = lifetime.signal;
  let bound = false;
  let cleanupRequested = false;
  let cleaned = false;
  let unsub: () => void = () => undefined;

  const cleanup = () => {
    if (cleaned) return;
    if (!bound) {
      cleanupRequested = true;
      return;
    }
    cleaned = true;
    unsub();
    signal.removeEventListener("abort", onAbort);
    lifetime.abort();
  };
  const onAbort = () => {
    channel.close();
    cleanup();
  };

  if (signal.aborted) onAbort();
  else signal.addEventListener("abort", onAbort, { once: true });

  return {
    cleanup,
    close: () => {
      channel.close();
      cleanup();
    },
    fail: (error) => {
      channel.fail(error);
      cleanup();
    },
    bind: (nextUnsub) => {
      if (bound) throw new Error("stream lifecycle is already bound");
      bound = true;
      unsub = nextUnsub;
      if (cleanupRequested) cleanup();
    },
  };
}

interface StreamLifetime {
  /** Combined caller + stream-owned signal passed to the transport request. */
  signal: AbortSignal;
  /** End only this stream without mutating the caller-owned signal. */
  abort(): void;
}

function createStreamLifetime(parent?: AbortSignal): StreamLifetime {
  const controller = new AbortController();
  return {
    signal: parent ? AbortSignal.any([parent, controller.signal]) : controller.signal,
    abort: () => controller.abort(),
  };
}

// ---------------------------------------------------------------------------
// Run-event streams
// ---------------------------------------------------------------------------

/** A run-event stream plus its teardown. `dispose` exists for the case where
 *  the stream's owning call FAILS before anyone iterates `events` — without
 *  it the subscription (and, for the deferred variant, its grow-forever
 *  pre-bind buffer) leaks, since iterableOf's cleanup only runs on iteration. */
export interface RunEventStream {
  events: AsyncIterable<RunEvent>;
  /** Signal owned by this stream and passed to its opening RPC request. */
  requestSignal: AbortSignal;
  dispose: () => void;
}

export interface RunEventStreamOptions {
  signal?: AbortSignal;
  /** Exact retention window advertised by `capabilities.limits.runReplay`.
   *  When discovery is unavailable, the SDK applies a bounded local safety
   *  envelope without claiming a replay promise the Runtime never advertised. */
  replayLimits?: RunReplayLimits;
}

/**
 * Subscribe to run events BEFORE the root segment id is known, then bind once
 * `runs.start` / `runs.resume` / `runs.subscribe` returns. Under streamable
 * HTTP the call's response and its event frames arrive on one ordered stream
 * (TRANSPORT.md §6.4), so the head events land right after the response —
 * subscribing only after the response resolves races and drops them. So we
 * subscribe immediately, bind the transport-owned request id before send, buffer
 * that response stream's events until `bind(segmentId)` supplies the
 * runtime-assigned terminal identity, then replay the buffer in response order.
 * (Every stream-opening method returns its root segmentId, so this is the single
 * run-event stream builder — a Run's runId is stable, but the segment being
 * streamed is only known from the response.)
 */
export function streamRunEvents(
  client: RpcClient,
  options: RunEventStreamOptions = {},
): RunEventStream & {
  bindRequest: (requestRpcId: RpcId) => void;
  bind: (rootSegmentId: string) => void;
} {
  const lifetime = createStreamLifetime(options.signal);
  // Validate and snapshot advertised budgets before transport registrations
  // exist; a malformed SDK capability cannot leave a half-open stream behind.
  const inbox = new RunEventInbox(options.replayLimits);
  const channel = inbox.channel;
  // A root finish is the final event in its response stream. Until the ack
  // supplies the root identity, retaining the latest finished segment is
  // therefore sufficient and cannot grow with a wide subagent tree.
  let latestFinishedBeforeBind: string | undefined;
  let ownerRequestRpcId: RpcId | undefined;
  let response: BoundRunResponse | null = null;
  const lifecycle = createStreamLifecycle(channel, lifetime);

  const unsubEvents = client.subscribe(RUN_EVENT_METHOD, {
    next(event, requestRpcId) {
      if (channel.closed || requestRpcId !== ownerRequestRpcId) return;
      const admission = inbox.admit(event);
      if (admission === "authoritativeOverflow") {
        lifecycle.fail(
          new RpcConnectionError(
            "run event consumer exceeded its bounded inbox before an authoritative event",
          ),
        );
        return;
      }
      if (admission !== "accepted" || event.event.type !== "segment.finished") return;
      if (response === null) latestFinishedBeforeBind = event.segmentId;
      else if (response.isRootFinish(event)) lifecycle.close();
    },
    error: (error, requestRpcId) => {
      if (requestRpcId !== undefined && requestRpcId !== ownerRequestRpcId) return;
      lifecycle.fail(error);
    },
  });
  const unsubDown = client.onStreamEnd((event) => {
    if (channel.closed || event.requestRpcId !== ownerRequestRpcId) return;
    if (event.error) lifecycle.fail(event.error);
    else lifecycle.close();
  });

  const bind = (rootSegmentId: string): void => {
    if (response !== null) return;
    response = new BoundRunResponse(rootSegmentId);
    if (latestFinishedBeforeBind === rootSegmentId) lifecycle.close();
    latestFinishedBeforeBind = undefined;
  };

  lifecycle.bind(() => {
    unsubEvents();
    unsubDown();
  });
  return {
    events: iterableOf(channel, lifecycle.cleanup),
    requestSignal: lifetime.signal,
    bindRequest: (requestRpcId) => {
      if (ownerRequestRpcId !== undefined) {
        throw new Error("run event stream is already bound to a request");
      }
      ownerRequestRpcId = requestRpcId;
    },
    bind,
    dispose: lifecycle.close,
  };
}

// ---------------------------------------------------------------------------
// Runtime event stream
// ---------------------------------------------------------------------------

/** The runtime notification stream plus its teardown (see RunEventStream).
 *  Connection-scoped and lossy: no terminal frame, no replay — the stream
 *  ends when its POST stream does, reported as a typed transport stream-end event.
 *  The consumer resubscribes and treats reconnect as `resync`. */
export interface RuntimeEventStream {
  events: AsyncIterable<RuntimeEvent>;
  /** Signal owned by this stream and passed to its opening RPC request. */
  requestSignal: AbortSignal;
  dispose: () => void;
}

/** Connection-scoped invalidations are deliberately compact; sustained lag is
 * recovered by ending this generation and resubscribing with an explicit
 * resync, never by retaining an unbounded second invalidation log. */
export const MAXIMUM_BUFFERED_RUNTIME_EVENTS = 64;

export function streamRuntimeEvents(
  client: RpcClient,
  signal?: AbortSignal,
): RuntimeEventStream & { bindRequest: (requestRpcId: RpcId) => void } {
  const lifetime = createStreamLifetime(signal);
  const channel = createPushPullChannel<RuntimeEvent>({
    capacity: MAXIMUM_BUFFERED_RUNTIME_EVENTS,
  });
  let ownerRequestRpcId: RpcId | undefined;
  const lifecycle = createStreamLifecycle(channel, lifetime);
  const unsubEvents = client.subscribe(RUNTIME_EVENT_METHOD, {
    next(params, requestRpcId) {
      if (channel.closed || requestRpcId !== ownerRequestRpcId) return;
      if (!channel.tryPush(params.event)) {
        lifecycle.fail(new RpcConnectionError("runtime event consumer exceeded its bounded inbox"));
      }
    },
    error: (error, requestRpcId) => {
      if (requestRpcId !== undefined && requestRpcId !== ownerRequestRpcId) return;
      lifecycle.fail(error);
    },
  });
  const unsubDown = client.onStreamEnd((event) => {
    if (channel.closed) return;
    if (event.method !== RUNTIME_SUBSCRIBE_METHOD || event.requestRpcId !== ownerRequestRpcId) {
      return;
    }
    if (event.error) lifecycle.fail(event.error);
    else lifecycle.close();
  });
  lifecycle.bind(() => {
    unsubEvents();
    unsubDown();
  });
  return {
    events: iterableOf(channel, lifecycle.cleanup),
    requestSignal: lifetime.signal,
    bindRequest: (requestRpcId) => {
      if (ownerRequestRpcId !== undefined) {
        throw new Error("runtime event stream is already bound to a request");
      }
      ownerRequestRpcId = requestRpcId;
    },
    dispose: lifecycle.close,
  };
}
