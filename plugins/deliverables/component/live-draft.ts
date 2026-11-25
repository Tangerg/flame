import { canonicalJSON } from "@frame/sdk/json";

// A draft owns only unsubmitted intent; the native revision named by base
// remains the authority that every expect value was read from.
export interface Draft<Operation> {
  base: string;
  operations: Operation[];
  undo: Operation[][];
}

export type DraftUpdate<Operation> = (
  current: Draft<Operation> | undefined,
) => Draft<Operation> | undefined;

export type ReconciliationStatus = "clean" | "applied" | "conflict" | "missing";

export interface Reconciliation<Operation> {
  operation: Operation;
  status: ReconciliationStatus;
  // The operation re-addressed to the current revision, when it has a target there.
  rebased?: Operation;
  current?: unknown;
}

export const same = (left: unknown, right: unknown) =>
  canonicalJSON(left) === canonicalJSON(right);

export function edited<Operation>(
  draft: Draft<Operation>,
  operations: Operation[],
): Draft<Operation> {
  return {
    base: draft.base,
    operations,
    undo: [...draft.undo, draft.operations],
  };
}

export function undone<Operation>(draft: Draft<Operation>): Draft<Operation> {
  if (!draft.undo.length) return draft;
  return {
    base: draft.base,
    operations: draft.undo.at(-1)!,
    undo: draft.undo.slice(0, -1),
  };
}

// Keeping mine re-reads the current value as the new precondition, so the
// authoritative write still refuses anything that changes after this choice.
export function rebase<Operation>(
  head: string,
  reconciliations: Reconciliation<Operation>[],
  keepMine: ReadonlySet<number>,
  withCurrent: (operation: Operation, current: unknown) => Operation,
): Draft<Operation> {
  const operations = reconciliations.flatMap(
    ({ status, rebased, current }, index): Operation[] => {
      if (!rebased || status === "applied" || status === "missing") return [];
      if (status === "clean") return [rebased];
      return keepMine.has(index) ? [withCurrent(rebased, current)] : [];
    },
  );
  return { base: head, operations, undo: [] };
}

export function operationsJSON(operations: readonly unknown[]): string {
  return canonicalJSON({ operations });
}
