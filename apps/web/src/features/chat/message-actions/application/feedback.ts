import { tupleKey } from "@/lib/tupleKey";
import { GenerationRetiredError } from "@/lib/asyncOwnership";
import { RetirableTaskCohort } from "@/lib/taskQueue";
import { type FeedbackRating } from "@/features/chat/message-actions/domain/feedback";
export interface MessageFeedbackTarget {
  readonly sessionId: string;
  readonly messageId: string;
  readonly runId?: string;
}

export interface MessageFeedbackGateway {
  createMessageFeedback(input: {
    target: MessageFeedbackTarget;
    rating: FeedbackRating;
  }): Promise<void>;
}

type RatingListener = () => void;

class MessageFeedbackAggregate {
  readonly #target: MessageFeedbackTarget;
  readonly #gateway: MessageFeedbackGateway;
  readonly #cohort: RetirableTaskCohort;
  readonly #publish: () => void;
  #accepted: FeedbackRating | undefined;
  #selected: FeedbackRating | undefined;
  #latestLease: object = {};
  #latestResult: Promise<FeedbackRating> | null = null;

  constructor(
    target: MessageFeedbackTarget,
    gateway: MessageFeedbackGateway,
    cohort: RetirableTaskCohort,
    publish: () => void,
  ) {
    this.#target = { ...target };
    this.#gateway = gateway;
    this.#cohort = cohort;
    this.#publish = publish;
  }

  rating(): FeedbackRating | undefined {
    return this.#selected;
  }

  submit(rating: FeedbackRating): Promise<FeedbackRating> {
    this.#cohort.assertCurrent();
    if (this.#selected === rating) {
      return this.#latestResult ?? Promise.resolve(rating);
    }

    const lease = (this.#latestLease = {});
    this.#selected = rating;
    this.#publish();

    const result = this.#cohort.runSerial(messageFeedbackIdentity(this.#target), () =>
      this.#execute(lease, rating),
    );
    this.#latestResult = result;
    const release = () => {
      if (this.#latestResult === result) this.#latestResult = null;
    };
    void result.then(release, release);
    return result;
  }

  async #execute(lease: object, rating: FeedbackRating): Promise<FeedbackRating> {
    try {
      await this.#cohort.settle(
        this.#gateway.createMessageFeedback({ target: this.#target, rating }),
      );
      this.#cohort.assertCurrent();
      this.#accepted = rating;
      return rating;
    } catch (error) {
      if (!this.#cohort.retired && lease === this.#latestLease) {
        this.#selected = this.#accepted;
        this.#publish();
      }
      throw error;
    }
  }
}

class MessageFeedbackGeneration {
  readonly #gateway: MessageFeedbackGateway;
  readonly #publish: (identity: string) => void;
  readonly #retiredError = new GenerationRetiredError("message_feedback_generation");
  readonly #cohort = new RetirableTaskCohort(this.#retiredError);
  readonly #aggregates = new Map<string, MessageFeedbackAggregate>();

  constructor(gateway: MessageFeedbackGateway, publish: (identity: string) => void) {
    this.#gateway = gateway;
    this.#publish = publish;
  }

  rating(target: MessageFeedbackTarget): FeedbackRating | undefined {
    return this.#aggregates.get(messageFeedbackIdentity(target))?.rating();
  }

  async submit(target: MessageFeedbackTarget, rating: FeedbackRating): Promise<FeedbackRating> {
    this.#cohort.assertCurrent();
    const identity = messageFeedbackIdentity(target);
    let aggregate = this.#aggregates.get(identity);
    if (!aggregate) {
      aggregate = new MessageFeedbackAggregate(target, this.#gateway, this.#cohort, () => {
        this.#publish(identity);
      });
      this.#aggregates.set(identity, aggregate);
    }
    return aggregate.submit(rating);
  }

  retire(): void {
    this.#cohort.retire();
    this.#aggregates.clear();
  }
}

export type MessageFeedbackCommands = Pick<MessageFeedbackGeneration, "submit">;
export type MessageFeedbackAccess = Pick<
  MessageFeedbackOwner,
  "capture" | "subscribeGeneration" | "rating" | "subscribe"
>;

export class MessageFeedbackOwner {
  readonly #listeners = new Map<string, Set<RatingListener>>();
  readonly #generationListeners = new Set<() => void>();
  #generation: MessageFeedbackGeneration | null;
  #disposed = false;

  constructor(gateway: MessageFeedbackGateway | null) {
    this.#generation = gateway ? this.#newGeneration(gateway) : null;
  }

  readonly capture = (): MessageFeedbackCommands | null => this.#generation;

  readonly subscribeGeneration = (listener: () => void): (() => void) => {
    this.#generationListeners.add(listener);
    return () => this.#generationListeners.delete(listener);
  };

  rating(target: MessageFeedbackTarget): FeedbackRating | undefined {
    return this.#generation?.rating(target);
  }

  subscribe(target: MessageFeedbackTarget, listener: RatingListener): () => void {
    const identity = messageFeedbackIdentity(target);
    let listeners = this.#listeners.get(identity);
    if (!listeners) {
      listeners = new Set();
      this.#listeners.set(identity, listeners);
    }
    listeners.add(listener);
    return () => {
      listeners.delete(listener);
      if (listeners.size === 0) this.#listeners.delete(identity);
    };
  }

  replaceRuntimeGeneration(createGateway: () => MessageFeedbackGateway): void {
    if (this.#disposed) return;
    const successor = this.#newGeneration(createGateway());
    const predecessor = this.#generation;
    this.#generation = successor;
    predecessor?.retire();
    this.#publishAll();
  }

  retireRuntimeGeneration(): void {
    if (!this.#generation) return;
    const predecessor = this.#generation;
    this.#generation = null;
    predecessor.retire();
    this.#publishAll();
  }

  dispose(): void {
    if (this.#disposed) return;
    this.#disposed = true;
    this.retireRuntimeGeneration();
    this.#listeners.clear();
    this.#generationListeners.clear();
  }

  #publishAll(): void {
    for (const listeners of this.#listeners.values()) {
      for (const listener of listeners) listener();
    }
    for (const listener of this.#generationListeners) listener();
  }

  #publish(identity: string): void {
    for (const listener of this.#listeners.get(identity) ?? []) listener();
  }

  #newGeneration(gateway: MessageFeedbackGateway): MessageFeedbackGeneration {
    return new MessageFeedbackGeneration(gateway, (identity) => this.#publish(identity));
  }
}

function messageFeedbackIdentity(target: MessageFeedbackTarget): string {
  return tupleKey(target.sessionId, target.messageId);
}
