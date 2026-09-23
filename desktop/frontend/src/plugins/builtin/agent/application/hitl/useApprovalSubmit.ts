import { useCallback } from "react";
import type { ApprovalDecision, InterruptRef, RememberScope } from "../../domain/hitl";
import { WIRE_DECISION } from "./wireDecision";
import { useInterruptResume } from "./useInterruptResume";

export interface ApprovalSubmitOptions {
  editedArgs?: Record<string, unknown>;
  rememberScope?: RememberScope;
}

export interface ApprovalActions {
  approve: () => void;
  decline: () => void;
}

interface ApprovalActionEntry {
  owner: InterruptRef;
  actions: ApprovalActions;
}

class ApprovalActionRegistry {
  readonly #entries = new Map<string, ApprovalActionEntry>();

  register(owner: InterruptRef, actions: ApprovalActions): () => void {
    const key = this.#key(owner);
    const entry = { owner, actions };
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

const approvalActionRegistry = new ApprovalActionRegistry();

export function registerApprovalActions(ref: InterruptRef, actions: ApprovalActions): () => void {
  return approvalActionRegistry.register(ref, actions);
}

export function getApprovalActions(ref: InterruptRef): ApprovalActions | undefined {
  return approvalActionRegistry.find(ref);
}

export interface ApprovalSubmit {
  submit: (decision: ApprovalDecision, opts?: ApprovalSubmitOptions) => void;
  pending: ApprovalDecision | null;
  registerActions: (actions: ApprovalActions) => () => void;
}

export function useApprovalSubmit(rootRunId?: string, itemId?: string): ApprovalSubmit {
  const { pending, resume, sessionId } = useInterruptResume<ApprovalDecision>(rootRunId, itemId);

  const submit = useCallback(
    (decision: ApprovalDecision, opts?: ApprovalSubmitOptions) => {
      resume(
        decision,
        {
          type: "approval",
          decision: WIRE_DECISION[decision],
          ...(opts?.editedArgs ? { editedArgs: opts.editedArgs } : {}),
          ...(opts?.rememberScope ? { remember: { scope: opts.rememberScope } } : {}),
        },
        { decision },
      );
    },
    [resume],
  );

  const registerActions = useCallback(
    (actions: ApprovalActions) => {
      if (!rootRunId || !itemId) return () => undefined;
      return registerApprovalActions({ sessionId, rootRunId, itemId }, actions);
    },
    [itemId, rootRunId, sessionId],
  );

  return { submit, pending, registerActions };
}
