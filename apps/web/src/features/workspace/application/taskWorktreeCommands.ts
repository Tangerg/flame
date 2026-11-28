import {
  type FlameClient,
  type MutationPromise,
  mutationSettlementIsUnknown,
} from "@flame/runtime-client";
import { GenerationRetiredError } from "@/lib/asyncOwnership";
import { RetirableTaskCohort } from "@/lib/taskQueue";

type TaskWorktrees = FlameClient["workspaces"]["tasks"];
export type TaskWorktreeMutation = "create" | "apply" | "release";

type TaskWorktreeCommandStatus =
  | { phase: "idle" }
  | { phase: "sending" | "committed"; operation: TaskWorktreeMutation }
  | { phase: "unconfirmed"; operation: TaskWorktreeMutation; error: unknown }
  | { phase: "failed"; error: unknown };

type TaskWorktreeCommandState =
  | Exclude<TaskWorktreeCommandStatus, { phase: "unconfirmed" }>
  | (Extract<TaskWorktreeCommandStatus, { phase: "unconfirmed" }> & {
      retry: OpenMutation;
    });

type OpenMutation = () => MutationPromise<unknown>;

export class TaskWorktreeCommands {
  #target: RetirableTaskCohort | null = null;
  #state: TaskWorktreeCommandState = { phase: "idle" };
  readonly #listeners = new Set<() => void>();

  constructor(private readonly worktrees: () => TaskWorktrees) {}

  readonly snapshot = (): TaskWorktreeCommandStatus => this.#state;

  readonly subscribe = (listener: () => void): (() => void) => {
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  };

  readonly replaceTarget = (): void => {
    this.#target?.retire();
    this.#target = new RetirableTaskCohort(new GenerationRetiredError("task_worktree_target"));
    this.#publish({ phase: "idle" });
  };

  retireTarget(): void {
    this.#target?.retire();
    this.#target = null;
    this.#publish({ phase: "idle" });
  }

  restorePending(): boolean {
    if (!this.#target || this.#state.phase === "sending" || this.#state.phase === "unconfirmed")
      return false;
    try {
      const [pending] = this.worktrees().pendingCommands();
      if (!pending) return true;
      this.#publish({
        phase: "unconfirmed",
        operation: pending.operation,
        retry: pending.retry,
        error: undefined,
      });
    } catch (error) {
      this.#publish({ phase: "failed", error });
    }
    return false;
  }

  create(request: Parameters<TaskWorktrees["create"]>[0]): Promise<boolean> {
    return this.#submit("create", () => this.worktrees().create(request));
  }

  apply(request: Parameters<TaskWorktrees["apply"]>[0]): Promise<boolean> {
    return this.#submit("apply", () => this.worktrees().apply(request));
  }

  release(id: string): Promise<boolean> {
    return this.#submit("release", () => this.worktrees().release(id));
  }

  async recover(): Promise<boolean> {
    const state = this.#state;
    if (state.phase !== "unconfirmed") return false;
    const target = this.#target;
    const accepted = await this.#observe(state.operation, state.retry, state.retry);
    if (this.#target === target) this.restorePending();
    return accepted;
  }

  #submit(operation: TaskWorktreeMutation, open: OpenMutation): Promise<boolean> {
    if (!this.restorePending()) return Promise.resolve(false);
    return this.#observe(operation, open);
  }

  async #observe(
    operation: TaskWorktreeMutation,
    open: OpenMutation,
    retryOriginal?: OpenMutation,
  ): Promise<boolean> {
    const target = this.#target;
    if (!target) return false;
    this.#publish({ phase: "sending", operation });
    let receipt: MutationPromise<unknown> | undefined;
    try {
      await target.run(() => (receipt = open()));
      target.assertCurrent();
    } catch (error) {
      if (target.retired) return false;
      const original = receipt;
      const retry = original ? () => original.retry() : retryOriginal;
      if (retry && (!original || mutationSettlementIsUnknown(error))) {
        this.#publish({ phase: "unconfirmed", operation, retry, error });
      } else {
        this.#publish({ phase: "failed", error });
      }
      return false;
    }
    this.#publish({ phase: "committed", operation });
    return true;
  }

  #publish(state: TaskWorktreeCommandState): void {
    this.#state = state;
    for (const listener of this.#listeners) listener();
  }
}

export type TaskWorktreeCommandAccess = Pick<
  TaskWorktreeCommands,
  "snapshot" | "subscribe" | "restorePending" | "create" | "apply" | "release" | "recover"
>;
