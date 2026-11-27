type InactiveGoalMode = { readonly phase: "inactive"; readonly sessionId: null };
type EngagedGoalMode = { readonly phase: "armed" | "starting"; readonly sessionId: string };
type ConfirmingGoalMode = {
  readonly phase: "confirming";
  readonly sessionId: string;
  readonly replacedObjective: string;
};
type GoalComposerModeSnapshot = InactiveGoalMode | EngagedGoalMode | ConfirmingGoalMode;
type GoalComposerModeState =
  | InactiveGoalMode
  | EngagedGoalMode
  | (ConfirmingGoalMode & { readonly start: (submission: GoalComposerSubmission) => void });

export interface GoalComposerSubmission {
  started(): boolean;
  failed(): boolean;
}

const INACTIVE: InactiveGoalMode = { phase: "inactive", sessionId: null };

export class GoalComposerModeOwner {
  #state: GoalComposerModeState = INACTIVE;
  readonly #listeners = new Set<() => void>();
  #disposed = false;

  readonly subscribe = (listener: () => void): (() => void) => {
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  };

  readonly snapshot = (): GoalComposerModeSnapshot => this.#state;

  get retired(): boolean {
    return this.#disposed;
  }

  active(sessionId: string): boolean {
    return this.#state.sessionId === sessionId;
  }

  activate(sessionId: string): boolean {
    if (this.#disposed) return false;
    this.#publish({ sessionId, phase: "armed" });
    return true;
  }

  toggle(sessionId: string): boolean {
    if (this.#disposed) return false;
    if (this.#state.sessionId === sessionId && this.#state.phase === "armed") {
      return this.deactivate(sessionId);
    }
    if (this.#state.phase === "confirming" || this.#state.phase === "starting") return false;
    return this.activate(sessionId);
  }

  begin(sessionId: string): GoalComposerSubmission | null {
    if (this.#disposed || !this.active(sessionId) || this.#state.phase !== "armed") {
      return null;
    }
    return this.#begin(sessionId);
  }

  requestReplacement(
    sessionId: string,
    objective: string,
    start: (submission: GoalComposerSubmission) => void,
  ): boolean {
    if (this.#disposed || !this.active(sessionId) || this.#state.phase !== "armed") {
      return false;
    }
    this.#publish({ sessionId, phase: "confirming", replacedObjective: objective, start });
    return true;
  }

  confirmReplacement(sessionId: string): boolean {
    const state = this.#state;
    if (this.#disposed || state.sessionId !== sessionId || state.phase !== "confirming") {
      return false;
    }
    state.start(this.#begin(sessionId));
    return true;
  }

  cancelReplacement(sessionId: string): boolean {
    if (
      this.#disposed ||
      this.#state.sessionId !== sessionId ||
      this.#state.phase !== "confirming"
    ) {
      return false;
    }
    this.#publish({ sessionId, phase: "armed" });
    return true;
  }

  #begin(sessionId: string): GoalComposerSubmission {
    const snapshot: EngagedGoalMode = {
      sessionId,
      phase: "starting",
    };
    this.#publish(snapshot);
    const settle = (next: GoalComposerModeState) => {
      if (this.#disposed || this.#state !== snapshot) return false;
      this.#publish(next);
      return true;
    };
    return {
      started: () => settle(INACTIVE),
      failed: () => settle({ sessionId, phase: "armed" }),
    };
  }

  deactivate(sessionId: string): boolean {
    if (this.#disposed || this.#state.sessionId !== sessionId) return false;
    this.#publish(INACTIVE);
    return true;
  }

  reset(): void {
    if (this.#disposed) return;
    this.#publish(INACTIVE);
  }

  dispose(): void {
    if (this.#disposed) return;
    this.#disposed = true;
    this.#state = INACTIVE;
    for (const listener of this.#listeners) listener();
    this.#listeners.clear();
  }

  #publish(snapshot: GoalComposerModeState): void {
    if (this.#state === snapshot) return;
    this.#state = snapshot;
    for (const listener of this.#listeners) listener();
  }
}
