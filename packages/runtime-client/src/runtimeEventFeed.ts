import type {
  RuntimeEvent,
  RuntimeSubscribeRequest,
  RuntimeSubscribeResponse,
  RuntimeTopic,
} from "@flame/runtime-client/wire";
import { createPushPullChannel } from "./channel";
import { RpcConnectionError } from "./errors";
import { checkRequest } from "./request";
import { iterableOf, MAXIMUM_BUFFERED_RUNTIME_EVENTS, type StreamingResult } from "./stream";
import { RUNTIME_SUBSCRIBE_METHOD } from "./transport";

type RuntimeEventSubscription = StreamingResult<RuntimeSubscribeResponse, RuntimeEvent>;

type OpenRuntimeEvents = (
  request: RuntimeSubscribeRequest,
  signal?: AbortSignal,
) => Promise<RuntimeEventSubscription>;

class Subscriber {
  readonly #channel = createPushPullChannel<RuntimeEvent>({
    capacity: MAXIMUM_BUFFERED_RUNTIME_EVENTS,
  });
  readonly #cancellation: PromiseWithResolvers<never> | undefined;
  #sequence = 0;

  constructor(
    private readonly topics: ReadonlySet<RuntimeTopic>,
    private readonly signal: AbortSignal | undefined,
    private readonly leave: (subscriber: Subscriber) => void,
  ) {
    this.#cancellation = signal && Promise.withResolvers<never>();
  }

  async open(
    acknowledgement: Promise<RuntimeSubscribeResponse>,
  ): Promise<RuntimeEventSubscription> {
    this.signal?.addEventListener("abort", this.#abort, { once: true });
    try {
      // Other subscribers can keep the shared opening alive after this observer leaves.
      const result = await (this.#cancellation
        ? Promise.race([acknowledgement, this.#cancellation.promise])
        : acknowledgement);
      this.signal?.throwIfAborted();
      return { result, events: iterableOf(this.#channel, this.#detach, this.signal) };
    } catch (error) {
      this.#cancel();
      throw error;
    }
  }

  fail(error: unknown): void {
    this.#channel.fail(error);
    this.#detach();
  }

  #abort = () => {
    this.#cancellation?.reject(this.signal?.reason);
    this.#cancel();
  };

  #cancel(): void {
    this.#channel.cancel();
    this.#detach();
  }

  #detach = () => {
    this.signal?.removeEventListener("abort", this.#abort);
    this.leave(this);
  };

  // A dedicated subscription numbers only the frames it receives, so a shared
  // stream renumbers each subscriber's subset to keep gaps meaning loss.
  deliver(event: RuntimeEvent): boolean {
    const routed = this.#route(event);
    return routed === undefined || this.#channel.tryPush({ ...routed, sequence: ++this.#sequence });
  }

  #route(event: RuntimeEvent): RuntimeEvent | undefined {
    if (event.type !== "resync") return this.topics.has(event.type) ? event : undefined;
    const missed = event.topics.filter((topic) => this.topics.has(topic));
    return missed.length > 0
      ? { type: "resync", sequence: event.sequence, topics: missed }
      : undefined;
  }
}

class SharedStream {
  readonly #subscribers = new Set<Subscriber>();
  readonly #controller = new AbortController();
  readonly #opened: Promise<RuntimeSubscribeResponse>;

  constructor(
    topics: readonly RuntimeTopic[],
    open: OpenRuntimeEvents,
    private readonly ended: (stream: SharedStream) => void,
  ) {
    this.#opened = open({ topics: [...topics] }, this.#controller.signal).then(
      ({ result, events }) => {
        void this.#forward(events);
        return result;
      },
    );
  }

  join(topics: readonly RuntimeTopic[], signal?: AbortSignal): Promise<RuntimeEventSubscription> {
    const subscriber = new Subscriber(new Set(topics), signal, this.#leave);
    this.#subscribers.add(subscriber);
    return subscriber.open(this.#opened);
  }

  #leave = (subscriber: Subscriber) => {
    if (!this.#subscribers.delete(subscriber) || this.#subscribers.size > 0) return;
    this.ended(this);
    this.#controller.abort();
  };

  async #forward(events: AsyncIterable<RuntimeEvent>): Promise<void> {
    let failure: unknown = new RpcConnectionError("runtime subscription ended unexpectedly");
    try {
      for await (const event of events) {
        for (const subscriber of this.#subscribers) {
          if (!subscriber.deliver(event)) {
            subscriber.fail(
              new RpcConnectionError("runtime event consumer exceeded its bounded inbox"),
            );
          }
        }
      }
    } catch (error) {
      failure = error;
    }
    for (const subscriber of this.#subscribers) subscriber.fail(failure);
  }
}

// Browsers allow six HTTP/1.1 connections per origin and each subscription holds
// one open, so subscribers without file watches share one stream over every
// negotiated topic instead of exhausting the connections their requests need.
export function createRuntimeEventFeed(
  open: OpenRuntimeEvents,
  negotiatedTopics: () => readonly RuntimeTopic[] | undefined,
): { subscribe: OpenRuntimeEvents } {
  let current: SharedStream | undefined;
  const ended = (stream: SharedStream) => {
    if (current === stream) current = undefined;
  };
  return {
    async subscribe(input, signal) {
      const request = checkRequest(RUNTIME_SUBSCRIBE_METHOD, input);
      const topics = negotiatedTopics();
      if (
        request.watches?.length ||
        topics === undefined ||
        !request.topics.every((topic) => topics.includes(topic))
      )
        return open(request, signal);
      signal?.throwIfAborted();
      const stream = (current ??= new SharedStream(topics, open, ended));
      return stream.join(request.topics, signal);
    },
  };
}
