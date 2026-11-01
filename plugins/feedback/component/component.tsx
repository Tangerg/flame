import { useSyncExternalStore } from "react";
import {
  defineComponent,
  failureText,
  useTranslator,
  type ComponentContext,
} from "@flame/component-sdk";
import {
  asItemId,
  asRunId,
  asSessionId,
  mutationSettlementIsUnknown,
  type MutationPromise,
} from "@flame/runtime-client";
import type { FeedbackRating } from "@flame/runtime-client/wire";
import { IconButton } from "@flame/ui";
import { ThumbsUp, ThumbsDown } from "@flame/icons/react";
import { catalogs } from "./catalogs";

type Attempt = { rating: FeedbackRating; command: MutationPromise<void> };
type Selection = {
  accepted?: FeedbackRating;
  busy?: boolean;
  unresolved?: Attempt;
  failure?: unknown;
};
const idle: Selection = Object.freeze({});

class MessageRatings {
  private readonly selections = new Map<string, Selection>();
  private readonly listeners = new Set<() => void>();
  constructor(private readonly signal: AbortSignal) {}
  read(key: string) {
    return this.selections.get(key) ?? idle;
  }
  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  };
  private publish(key: string, selection: Selection) {
    if (this.signal.aborted) return;
    this.selections.set(key, selection);
    this.listeners.forEach((listener) => listener());
  }
  async submit(
    key: string,
    rating: FeedbackRating,
    create: () => MutationPromise<void>,
  ) {
    const { accepted, busy, unresolved } = this.read(key);
    if (
      this.signal.aborted ||
      busy ||
      (unresolved ? unresolved.rating !== rating : accepted === rating)
    )
      return;
    this.publish(key, { accepted, busy: true });
    let attempt = unresolved;
    try {
      const command = unresolved ? unresolved.command.retry() : create();
      attempt = { rating, command };
      await command;
      this.publish(key, { accepted: rating });
    } catch (failure) {
      this.publish(key, {
        accepted,
        failure,
        unresolved: mutationSettlementIsUnknown(failure) ? attempt : undefined,
      });
    }
  }
}

export const RateResponse = defineComponent((source) => {
  const ratings = new MessageRatings(source.signal);
  return function FeedbackAction({ context }: { context: ComponentContext }) {
    const message = context.message;
    const binding = context.binding;
    const t = useTranslator(catalogs, context.locale);
    const key = JSON.stringify([
      message?.sessionId,
      message?.runId,
      message?.itemId,
    ]);
    const state = useSyncExternalStore(ratings.subscribe, () =>
      ratings.read(key),
    );
    if (!message || message.role !== "assistant" || !binding) return null;
    return (
      <>
        {(["positive", "negative"] as const).map((rating) => (
          <IconButton
            key={rating}
            size="sm"
            quiet
            icon={rating === "positive" ? ThumbsUp : ThumbsDown}
            aria-label={t(rating === "positive" ? "good" : "poor")}
            aria-pressed={state.accepted === rating}
            disabled={
              state.busy ||
              Boolean(state.unresolved && state.unresolved.rating !== rating)
            }
            onClick={() =>
              void ratings.submit(key, rating, () =>
                context.runtime.feedback.create({
                  source: {installationId:binding.installationId,release:binding.digest,contribution:binding.contribution},
                  sessionId: asSessionId(message.sessionId),
                  runId: message.runId ? asRunId(message.runId) : undefined,
                  itemId: asItemId(message.itemId),
                  rating,
                }),
              )
            }
          />
        ))}
        {state.failure != null && (
          <span role="alert">{failureText(state.failure, t("failed"))}</span>
        )}
      </>
    );
  };
});
