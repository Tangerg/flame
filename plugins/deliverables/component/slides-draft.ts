import { canonicalJSON } from "@frame/sdk/json";
import type {
  ChartData,
  ElementView,
  Frame,
  SlideLayout,
} from "../contracts/content";

export type DraftOperation =
  | { type: "setFrame"; id: string; expect: Frame; frame: Frame }
  | { type: "replaceText"; id: string; expect: string; text: string }
  | { type: "deleteElement"; id: string }
  | { type: "setChartData"; id: string; expect: ChartData; data: ChartData };

// The draft owns only unsubmitted intent; the native revision named by base
// remains the authority that every expect value was read from.
export interface Draft {
  base: string;
  operations: DraftOperation[];
  undo: DraftOperation[][];
}

const same = (left: unknown, right: unknown) =>
  canonicalJSON(left) === canonicalJSON(right);

function target(operation: DraftOperation): unknown {
  switch (operation.type) {
    case "setFrame":
      return operation.frame;
    case "replaceText":
      return operation.text;
    case "setChartData":
      return operation.data;
    case "deleteElement":
      return undefined;
  }
}

function expected(operation: DraftOperation): unknown {
  return "expect" in operation ? operation.expect : undefined;
}

export function record(
  draft: Draft,
  next: DraftOperation,
  element?: ElementView,
): Draft {
  let operations = draft.operations;
  if (next.type === "deleteElement" && element) {
    const runs = new Set(element.text.map((run) => run.id));
    operations = operations.filter(
      (operation) => operation.id !== element.id && !runs.has(operation.id),
    );
  }
  const prior = operations.find(
    (operation) => operation.type === next.type && operation.id === next.id,
  );
  const merged = (
    prior && "expect" in prior ? { ...next, expect: prior.expect } : next
  ) as DraftOperation;
  const remaining = operations.filter((operation) => operation !== prior);
  const unchanged =
    merged.type !== "deleteElement" && same(expected(merged), target(merged));
  return {
    base: draft.base,
    operations: unchanged ? remaining : [...remaining, merged],
    undo: [...draft.undo, draft.operations],
  };
}

export function revert(draft: Draft, element: ElementView): Draft {
  const runs = new Set(element.text.map((run) => run.id));
  return {
    base: draft.base,
    operations: draft.operations.filter(
      (operation) => operation.id !== element.id && !runs.has(operation.id),
    ),
    undo: [...draft.undo, draft.operations],
  };
}

export function preview(
  layout: SlideLayout,
  operations: DraftOperation[],
): SlideLayout {
  const by = <T extends DraftOperation["type"]>(type: T) =>
    new Map(
      operations
        .filter(
          (operation): operation is Extract<DraftOperation, { type: T }> =>
            operation.type === type,
        )
        .map((operation) => [operation.id, operation]),
    );
  const frames = by("setFrame");
  const texts = by("replaceText");
  const charts = by("setChartData");
  const deleted = by("deleteElement");
  return {
    ...layout,
    slides: layout.slides.map((slide) => ({
      ...slide,
      notes: slide.notes.map((run) => ({
        ...run,
        text: texts.get(run.id)?.text ?? run.text,
      })),
      elements: slide.elements
        .filter((element) => !deleted.has(element.id))
        .map((element) => ({
          ...element,
          ...(frames.has(element.id)
            ? {
                frame: frames.get(element.id)!.frame,
                frameSource: "slide" as const,
              }
            : {}),
          text: element.text.map((run) => ({
            ...run,
            text: texts.get(run.id)?.text ?? run.text,
          })),
          ...(element.chart && charts.has(element.id)
            ? {
                chart: {
                  ...element.chart,
                  data: charts.get(element.id)!.data,
                },
              }
            : {}),
        })),
    })),
  };
}

// Text run identities are positions within one native part, so a run is
// matched through its element (or the slide's notes) and its order there.
function runOwners(layout: SlideLayout) {
  const owners = new Map<string, { owner: string; index: number }>();
  for (const slide of layout.slides) {
    slide.notes.forEach((run, index) =>
      owners.set(run.id, { owner: `${slide.id}#notes`, index }),
    );
    for (const element of slide.elements)
      element.text.forEach((run, index) =>
        owners.set(run.id, { owner: element.id, index }),
      );
  }
  return owners;
}

function runsOf(layout: SlideLayout, owner: string) {
  for (const slide of layout.slides) {
    if (owner === `${slide.id}#notes`) return slide.notes;
    const element = slide.elements.find((entry) => entry.id === owner);
    if (element) return element.text;
  }
}

function elementOf(layout: SlideLayout, id: string) {
  for (const slide of layout.slides) {
    const element = slide.elements.find((entry) => entry.id === id);
    if (element) return element;
  }
}

export type Reconciliation = {
  operation: DraftOperation;
  status: "clean" | "applied" | "conflict" | "missing";
  // The operation re-addressed to the current revision, when it has a target there.
  rebased?: DraftOperation;
  current?: unknown;
};

export function reconcile(
  base: SlideLayout,
  head: SlideLayout,
  operations: DraftOperation[],
): Reconciliation[] {
  const baseRuns = runOwners(base);
  return operations.map((operation): Reconciliation => {
    if (operation.type === "replaceText") {
      const owner = baseRuns.get(operation.id);
      const run = owner && runsOf(head, owner.owner)?.[owner.index];
      if (!run) return { operation, status: "missing" };
      const rebased = { ...operation, id: run.id };
      if (run.text === operation.text)
        return { operation, status: "applied", rebased };
      if (run.text === operation.expect)
        return { operation, status: "clean", rebased };
      return { operation, status: "conflict", rebased, current: run.text };
    }
    const current = elementOf(head, operation.id);
    if (operation.type === "deleteElement") {
      if (!current) return { operation, status: "applied" };
      return same(current, elementOf(base, operation.id))
        ? { operation, status: "clean", rebased: operation }
        : { operation, status: "conflict", rebased: operation, current };
    }
    const value =
      operation.type === "setFrame" ? current?.frame : current?.chart?.data;
    if (!current || value === undefined)
      return { operation, status: "missing" };
    if (same(value, target(operation)))
      return { operation, status: "applied", rebased: operation };
    if (same(value, operation.expect))
      return { operation, status: "clean", rebased: operation };
    return {
      operation,
      status: "conflict",
      rebased: operation,
      current: value,
    };
  });
}

// Keeping mine re-reads the current value as the new precondition, so the
// authoritative write still refuses anything that changes after this choice.
export function rebase(
  head: string,
  reconciliations: Reconciliation[],
  keepMine: ReadonlySet<number>,
): Draft {
  const operations = reconciliations.flatMap(
    ({ status, rebased, current }, index): DraftOperation[] => {
      if (!rebased || status === "applied" || status === "missing") return [];
      if (status === "clean") return [rebased];
      if (!keepMine.has(index)) return [];
      if (rebased.type === "deleteElement") return [rebased];
      return [{ ...rebased, expect: current } as DraftOperation];
    },
  );
  return { base: head, operations, undo: [] };
}

export function operationsJSON(operations: DraftOperation[]): string {
  return canonicalJSON({ operations });
}
