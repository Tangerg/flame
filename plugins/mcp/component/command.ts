import type { ComponentContext } from "@flame/component-sdk";
import type { MutationPromise } from "@flame/runtime-client";

export type SettingsContext = Pick<
  ComponentContext,
  "component" | "runtime" | "signal" | "track"
>;

export interface CommandStep {
  readonly resource: string;
  execute(context: SettingsContext): MutationPromise<unknown>;
  accept?(context: SettingsContext, value: unknown): void;
}

type Cursor =
  | { phase: "ready"; index: number }
  | { phase: "observing"; index: number; original: MutationPromise<unknown> };

export class CommandBatch {
  #cursor: Cursor = { phase: "ready", index: 0 };

  constructor(private readonly steps: readonly CommandStep[]) {}

  get progress() {
    return { completed: this.#cursor.index, total: this.steps.length };
  }

  get complete() {
    return this.#cursor.index === this.steps.length;
  }

  get hasOriginal() {
    return this.#cursor.phase === "observing";
  }

  someRemainingResource(matches: (resource: string) => boolean) {
    return this.steps
      .slice(this.#cursor.index)
      .some(({ resource }) => matches(resource));
  }

  observe(context: SettingsContext) {
    context.signal.throwIfAborted();
    const cursor = this.#cursor;
    if (cursor.phase === "observing")
      return context.track(() =>
        cursor.original.retry({ signal: context.signal }),
      );

    const step = this.steps[cursor.index]!;
    const original = context.track(() => step.execute(context));
    this.#cursor = { phase: "observing", index: cursor.index, original };
    return original;
  }

  accept(context: SettingsContext, value: unknown) {
    const { index } = this.#cursor;
    this.#cursor = { phase: "ready", index: index + 1 };
    this.steps[index]!.accept?.(context, value);
  }

  releaseOriginal() {
    this.#cursor = { phase: "ready", index: this.#cursor.index };
  }
}
