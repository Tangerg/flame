type StartingGoal = { readonly phase: "starting" };
type GoalReplacement = { readonly phase: "confirming"; readonly replacedObjective: string };
type GoalSubmissionSnapshot = StartingGoal | GoalReplacement;

export type CompleteGoalSubmission = () => boolean;

type ConfirmingGoal = GoalReplacement & {
  readonly start: (complete: CompleteGoalSubmission) => void;
};

export class GoalComposerSubmissionOwner {
  readonly #pending = new Map<string, StartingGoal | ConfirmingGoal>();
  readonly #listeners = new Set<() => void>();
  #disposed = false;

  readonly subscribe = (listener: () => void): (() => void) => {
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  };

  snapshot(sessionId: string): GoalSubmissionSnapshot | null {
    return this.#pending.get(sessionId) ?? null;
  }

  begin(sessionId: string): CompleteGoalSubmission | null {
    if (this.#disposed || this.#pending.has(sessionId)) return null;
    return this.#begin(sessionId);
  }

  requestReplacement(
    sessionId: string,
    replacedObjective: string,
    start: ConfirmingGoal["start"],
  ): void {
    if (this.#disposed || this.#pending.has(sessionId)) return;
    this.#pending.set(sessionId, { phase: "confirming", replacedObjective, start });
    this.#notify();
  }

  confirmReplacement(sessionId: string): void {
    const pending = this.#pending.get(sessionId);
    if (pending?.phase === "confirming") pending.start(this.#begin(sessionId));
  }

  cancelReplacement(sessionId: string): void {
    if (this.#pending.get(sessionId)?.phase !== "confirming") return;
    this.#pending.delete(sessionId);
    this.#notify();
  }

  reset(): void {
    if (this.#pending.size === 0) return;
    this.#pending.clear();
    this.#notify();
  }

  dispose(): void {
    this.#disposed = true;
    this.reset();
    this.#listeners.clear();
  }

  #begin(sessionId: string): CompleteGoalSubmission {
    const pending: StartingGoal = { phase: "starting" };
    this.#pending.set(sessionId, pending);
    this.#notify();
    return () => {
      if (this.#pending.get(sessionId) !== pending) return false;
      this.#pending.delete(sessionId);
      this.#notify();
      return true;
    };
  }

  #notify(): void {
    for (const listener of this.#listeners) listener();
  }
}
