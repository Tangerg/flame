import { type InterruptRef } from "@/features/agent/domain/hitl";

export interface ApprovalActions {
  approve: () => void;
  decline: () => void;
}

// A mounted approval card registers its own submit so a keyboard decision
// commits the card's edited arguments exactly as a click would.
export class ApprovalActionRegistry {
  readonly #entries = new Map<string, { actions: ApprovalActions }>();

  register(owner: InterruptRef, actions: ApprovalActions): () => void {
    const key = this.#key(owner);
    const entry = { actions };
    this.#entries.set(key, entry);
    return () => {
      if (this.#entries.get(key) === entry) this.#entries.delete(key);
    };
  }

  find(owner: InterruptRef): ApprovalActions | undefined {
    return this.#entries.get(this.#key(owner))?.actions;
  }

  #key(owner: InterruptRef): string {
    return JSON.stringify([owner.sessionId, owner.rootRunId, owner.itemId]);
  }
}
