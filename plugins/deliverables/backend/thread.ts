import { LIMITS } from "../contracts/limits";
import {
  schemas,
  type ThreadMessage,
  type ThreadValue,
} from "../contracts/capability";
import { refusal } from "./refusal";

// A comment thread owns its anchor, messages and status. Replies never change
// the status; only an explicit resolve or reopen does.
export class Thread {
  readonly #value: ThreadValue;

  constructor(value: ThreadValue) {
    this.#value = Object.freeze(schemas.thread.parse(value));
  }

  static open(
    id: string,
    documentId: string,
    anchor: ThreadValue["anchor"],
    message: ThreadMessage,
  ): Thread {
    return new Thread({
      id,
      documentId,
      anchor,
      status: "open",
      createdAt: message.createdAt,
      updatedAt: message.createdAt,
      messages: [message],
    });
  }

  snapshot(): ThreadValue {
    return structuredClone(this.#value);
  }

  reply(message: ThreadMessage): Thread {
    if (this.#value.messages.length >= LIMITS.threadMessages)
      throw refusal(
        "thread_capacity",
        "the comment thread holds its maximum number of messages",
      );
    return new Thread({
      ...this.snapshot(),
      updatedAt: message.createdAt,
      messages: [...this.#value.messages, message],
    });
  }

  resolve(at: string): Thread {
    return this.#status("open", "resolved", at);
  }

  reopen(at: string): Thread {
    return this.#status("resolved", "open", at);
  }

  #status(
    from: ThreadValue["status"],
    to: ThreadValue["status"],
    at: string,
  ): Thread {
    if (this.#value.status !== from)
      throw refusal("thread_state", `the comment thread is already ${to}`);
    return new Thread({ ...this.snapshot(), status: to, updatedAt: at });
  }
}
