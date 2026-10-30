type GoalComposerModePhase = "inactive" | "armed" | "confirming" | "starting";

export interface GoalComposerModeSnapshot {
  readonly sessionId: string | null;
  readonly phase: GoalComposerModePhase;
  readonly replacedObjective: string | null;
}

export interface GoalComposerSubmission {
  finish(succeeded: boolean): boolean;
}

const INACTIVE: GoalComposerModeSnapshot = {
  sessionId: null,
  phase: "inactive",
  replacedObjective: null,
};

export class GoalComposerModeOwner {
  #snapshot: GoalComposerModeSnapshot = INACTIVE;
  #replacement: ((submission: GoalComposerSubmission) => void) | null = null;
  #listeners = new Set<() => void>();
  #disposed = false;

  readonly subscribe = (listener: () => void): (() => void) => {
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  };

  readonly snapshot = (): GoalComposerModeSnapshot => this.#snapshot;

  get retired(): boolean {
    return this.#disposed;
  }

  active(sessionId: string): boolean {
    return this.#snapshot.sessionId === sessionId && this.#snapshot.phase !== "inactive";
  }

  activate(sessionId: string): boolean {
    if (this.#disposed) return false;
    this.#replacement = null;
    this.#publish({ sessionId, phase: "armed", replacedObjective: null });
    return true;
  }

  toggle(sessionId: string): boolean {
    if (this.#disposed) return false;
    if (this.#snapshot.sessionId === sessionId && this.#snapshot.phase === "armed") {
      return this.deactivate(sessionId);
    }
    if (this.#snapshot.phase === "confirming" || this.#snapshot.phase === "starting") return false;
    return this.activate(sessionId);
  }

  begin(sessionId: string): GoalComposerSubmission | null {
    if (this.#disposed || !this.active(sessionId) || this.#snapshot.phase !== "armed") {
      return null;
    }
    return this.#begin(sessionId);
  }

  requestReplacement(
    sessionId: string,
    objective: string,
    start: (submission: GoalComposerSubmission) => void,
  ): boolean {
    if (this.#disposed || !this.active(sessionId) || this.#snapshot.phase !== "armed") {
      return false;
    }
    this.#replacement = start;
    this.#publish({ sessionId, phase: "confirming", replacedObjective: objective });
    return true;
  }

  confirmReplacement(sessionId: string): boolean {
    if (
      this.#disposed ||
      this.#snapshot.sessionId !== sessionId ||
      this.#snapshot.phase !== "confirming" ||
      !this.#replacement
    ) {
      return false;
    }
    const start = this.#replacement;
    this.#replacement = null;
    start(this.#begin(sessionId));
    return true;
  }

  cancelReplacement(sessionId: string): boolean {
    if (
      this.#disposed ||
      this.#snapshot.sessionId !== sessionId ||
      this.#snapshot.phase !== "confirming"
    ) {
      return false;
    }
    this.#replacement = null;
    this.#publish({ sessionId, phase: "armed", replacedObjective: null });
    return true;
  }

  #begin(sessionId: string): GoalComposerSubmission {
    const snapshot: GoalComposerModeSnapshot = {
      sessionId,
      phase: "starting",
      replacedObjective: null,
    };
    this.#publish(snapshot);
    return {
      finish: (succeeded) => {
        if (this.#disposed || this.#snapshot !== snapshot) return false;
        this.#publish(
          succeeded ? INACTIVE : { sessionId, phase: "armed", replacedObjective: null },
        );
        return true;
      },
    };
  }

  deactivate(sessionId: string): boolean {
    if (this.#disposed || this.#snapshot.sessionId !== sessionId) return false;
    this.#replacement = null;
    this.#publish(INACTIVE);
    return true;
  }

  reset(): void {
    if (this.#disposed) return;
    this.#replacement = null;
    this.#publish(INACTIVE);
  }

  dispose(): void {
    if (this.#disposed) return;
    this.#disposed = true;
    this.#replacement = null;
    this.#snapshot = INACTIVE;
    for (const listener of this.#listeners) listener();
    this.#listeners.clear();
  }

  #publish(snapshot: GoalComposerModeSnapshot): void {
    if (this.#snapshot === snapshot) return;
    this.#snapshot = snapshot;
    for (const listener of this.#listeners) listener();
  }
}
