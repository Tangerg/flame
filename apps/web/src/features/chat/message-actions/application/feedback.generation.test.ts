import { afterEach, describe, expect, it, onTestFinished, vi } from "vitest";
import {
  MessageFeedbackOwner,
  type MessageFeedbackGateway,
  type MessageFeedbackTarget,
} from "@/features/chat/message-actions/application/feedback";
import { rejected } from "@/test/rejected";
let owner: MessageFeedbackOwner | undefined;

afterEach(() => {
  owner?.dispose();
  owner = undefined;
});

describe("message feedback generation", () => {
  it("keeps ratings and subscriptions within their explicit instance", async () => {
    const gateway = { createMessageFeedback: vi.fn().mockResolvedValue(undefined) };
    owner = new MessageFeedbackOwner(gateway);
    const independent = new MessageFeedbackOwner(gateway);
    onTestFinished(() => independent.dispose());
    const target = feedbackTarget("independent");
    const changed = vi.fn();
    const unsubscribe = independent.subscribe(target, changed);
    onTestFinished(unsubscribe);
    await owner.capture()!.submit(target, "positive");
    expect(independent.rating(target)).toBeUndefined();
    expect(changed).not.toHaveBeenCalled();

    await independent.capture()!.submit(target, "negative");
    expect(independent.rating(target)).toBe("negative");
    expect(changed).toHaveBeenCalledOnce();
    owner.dispose();
    expect(independent.rating(target)).toBe("negative");
    expect(changed).toHaveBeenCalledOnce();
  });

  it("rejects captured feedback after replacement without submitting through the successor", async () => {
    const createMessageFeedback = vi.fn().mockResolvedValue(undefined);
    const successor = vi.fn().mockResolvedValue(undefined);
    owner = new MessageFeedbackOwner({ createMessageFeedback });
    const commands = owner.capture()!;
    const captured = commands.submit.bind(commands);
    owner.replaceRuntimeGeneration(() => ({ createMessageFeedback: successor }));

    await expect(captured(feedbackTarget("captured"), "positive")).rejects.toMatchObject({
      message: "message_feedback_generation_retired",
    });
    expect(createMessageFeedback).not.toHaveBeenCalled();
    expect(successor).not.toHaveBeenCalled();
  });

  it("serializes re-rating and keeps a newer accepted rating after an older failure", async () => {
    const positive = Promise.withResolvers<void>();
    const createMessageFeedback = vi.fn(({ rating }: { rating: string }) =>
      rating === "positive" ? positive.promise : Promise.resolve(),
    );
    owner = new MessageFeedbackOwner({ createMessageFeedback });
    const target = feedbackTarget("reordered");

    const older = rejected(owner!.capture()!.submit(target, "positive"));
    const newer = owner!.capture()!.submit(target, "negative");
    await vi.waitFor(() => expect(createMessageFeedback).toHaveBeenCalledTimes(1));
    expect(owner!.rating(target)).toBe("negative");

    positive.reject(new Error("older feedback failed late"));
    await expect(older).resolves.toMatchObject({ message: "older feedback failed late" });
    await expect(newer).resolves.toBe("negative");
    expect(createMessageFeedback.mock.calls.map(([input]) => input.rating)).toEqual([
      "positive",
      "negative",
    ]);
    expect(owner!.rating(target)).toBe("negative");
  });

  it("rolls the latest failed intent back to the last accepted rating", async () => {
    const negative = Promise.withResolvers<void>();
    owner = new MessageFeedbackOwner({
      createMessageFeedback: vi.fn(({ rating }) =>
        rating === "negative" ? negative.promise : Promise.resolve(),
      ),
    });
    const target = feedbackTarget("rollback");
    await owner!.capture()!.submit(target, "positive");

    const failed = rejected(owner!.capture()!.submit(target, "negative"));
    expect(owner!.rating(target)).toBe("negative");
    negative.reject(new Error("negative rejected"));

    await expect(failed).resolves.toMatchObject({ message: "negative rejected" });
    expect(owner!.rating(target)).toBe("positive");
  });

  it("retires in-flight and queued commands before a Runtime replacement can publish material", async () => {
    const retired = Promise.withResolvers<void>();
    const createMessageFeedback = vi.fn(() => retired.promise);
    owner = new MessageFeedbackOwner({ createMessageFeedback });
    const target = feedbackTarget("runtime_replacement");
    const listener = vi.fn();
    const unsubscribe = owner!.subscribe(target, listener);

    const inFlight = rejected(owner!.capture()!.submit(target, "positive"));
    const queued = rejected(owner!.capture()!.submit(target, "negative"));
    await vi.waitFor(() => expect(createMessageFeedback).toHaveBeenCalledOnce());
    owner.replaceRuntimeGeneration(() => ({ createMessageFeedback }));

    await expect(inFlight).resolves.toMatchObject({
      message: "message_feedback_generation_retired",
    });
    await expect(queued).resolves.toMatchObject({
      message: "message_feedback_generation_retired",
    });
    expect(owner!.rating(target)).toBeUndefined();
    const notificationsAtReplacement = listener.mock.calls.length;

    retired.resolve();
    await Promise.resolve();
    expect(createMessageFeedback).toHaveBeenCalledOnce();
    expect(owner!.rating(target)).toBeUndefined();
    expect(listener).toHaveBeenCalledTimes(notificationsAtReplacement);
    unsubscribe();
  });

  it("retires an admitted command immediately on final disposal", async () => {
    const pending = Promise.withResolvers<void>();
    owner = new MessageFeedbackOwner({
      createMessageFeedback: vi.fn(() => pending.promise),
    });
    const command = rejected(owner!.capture()!.submit(feedbackTarget("disposed"), "positive"));

    owner.dispose();
    await expect(command).resolves.toMatchObject({
      message: "message_feedback_generation_retired",
    });

    pending.resolve();
    await Promise.resolve();
  });

  it("replaces a Plugin Host owner without lending queued work to its successor", async () => {
    const retired = Promise.withResolvers<void>();
    const retiredGateway: MessageFeedbackGateway = {
      createMessageFeedback: vi.fn(() => retired.promise),
    };
    const successorGateway: MessageFeedbackGateway = {
      createMessageFeedback: vi.fn().mockResolvedValue(undefined),
    };
    const predecessor = new MessageFeedbackOwner(retiredGateway);
    owner = predecessor;
    const target = feedbackTarget("host_replacement");
    const inFlight = rejected(owner!.capture()!.submit(target, "positive"));
    const queued = rejected(owner!.capture()!.submit(target, "negative"));
    await vi.waitFor(() => expect(retiredGateway.createMessageFeedback).toHaveBeenCalledOnce());

    predecessor.dispose();
    owner = new MessageFeedbackOwner(successorGateway);
    await expect(inFlight).resolves.toMatchObject({
      message: "message_feedback_generation_retired",
    });
    await expect(queued).resolves.toMatchObject({
      message: "message_feedback_generation_retired",
    });
    expect(successorGateway.createMessageFeedback).not.toHaveBeenCalled();

    await expect(owner!.capture()!.submit(target, "negative")).resolves.toBe("negative");
    expect(successorGateway.createMessageFeedback).toHaveBeenCalledOnce();
    retired.resolve();
    predecessor.dispose();
  });

  it("scopes equal Item identities to their exact Session", async () => {
    const createMessageFeedback = vi.fn().mockResolvedValue(undefined);
    owner = new MessageFeedbackOwner({ createMessageFeedback });
    const first = feedbackTarget("shared", "ses_first");
    const second = feedbackTarget("shared", "ses_second");

    await owner!.capture()!.submit(first, "positive");

    expect(owner!.rating(first)).toBe("positive");
    expect(owner!.rating(second)).toBeUndefined();
  });
});

function feedbackTarget(suffix: string, sessionId = "ses_feedback"): MessageFeedbackTarget {
  return {
    sessionId,
    messageId: `item_${suffix}`,
    runId: "run_feedback",
  };
}
