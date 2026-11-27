import type { DraftOperation } from "../contracts/content";
import type { ElementView, SlideLayout } from "../contracts/layouts";
import {
  edited,
  fieldTarget,
  mergeDraftEdits,
  same,
  type Draft,
  type EditTarget,
  type Reconciliation,
} from "./live-draft";

export type SlideOperation = DraftOperation<"pptx">;

export function proposedValue(operation: SlideOperation): unknown {
  switch (operation.type) {
    case "setFrame":
      return operation.frame;
    case "replaceText":
      return operation.text;
    case "setChartData":
      return operation.data;
    case "setChartLegend":
      return operation.position;
    case "deleteElement":
      return undefined;
  }
}

function expected(operation: SlideOperation): unknown {
  return "expect" in operation ? operation.expect : undefined;
}

function editTarget(operation: SlideOperation, parent?: string): EditTarget {
  return operation.type === "deleteElement"
    ? { kind: "object", owner: operation.id }
    : fieldTarget(operation, parent);
}

export function mergeSuggestion(
  draft: Draft<SlideOperation>,
  layout: SlideLayout,
  offered: SlideOperation[],
): Draft<SlideOperation> {
  const owners = runOwners(layout);
  return mergeDraftEdits(draft, offered, (operation) =>
    editTarget(operation, owners.get(operation.id)?.owner),
  );
}

export function record(
  draft: Draft<SlideOperation>,
  next: SlideOperation,
  element?: ElementView,
): Draft<SlideOperation> {
  const runs = new Set(element?.text.map((run) => run.id));
  const prior = draft.operations.find(
    (operation) => operation.type === next.type && operation.id === next.id,
  );
  const merged = (
    prior && "expect" in prior ? { ...next, expect: prior.expect } : next
  ) as SlideOperation;
  const unchanged =
    merged.type !== "deleteElement" &&
    same(expected(merged), proposedValue(merged));
  if (unchanged)
    return prior
      ? edited(
          draft,
          draft.operations.filter((operation) => operation !== prior),
        )
      : draft;
  return mergeDraftEdits(draft, [merged], (operation) =>
    editTarget(operation, runs.has(operation.id) ? element?.id : undefined),
  );
}

export function revert(
  draft: Draft<SlideOperation>,
  element: ElementView,
): Draft<SlideOperation> {
  const runs = new Set(element.text.map((run) => run.id));
  return edited(
    draft,
    draft.operations.filter(
      (operation) => operation.id !== element.id && !runs.has(operation.id),
    ),
  );
}

export function preview(
  layout: SlideLayout,
  operations: SlideOperation[],
): SlideLayout {
  const by = <T extends SlideOperation["type"]>(type: T) =>
    new Map(
      operations
        .filter(
          (operation): operation is Extract<SlideOperation, { type: T }> =>
            operation.type === type,
        )
        .map((operation) => [operation.id, operation]),
    );
  const frames = by("setFrame");
  const texts = by("replaceText");
  const charts = by("setChartData");
  const legends = by("setChartLegend");
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
          ...(element.chart
            ? {
                chart: {
                  ...element.chart,
                  data: charts.get(element.id)?.data ?? element.chart.data,
                  ...(legends.has(element.id)
                    ? { legendPosition: legends.get(element.id)!.position }
                    : {}),
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

export function reconcile(
  base: SlideLayout,
  head: SlideLayout,
  operations: SlideOperation[],
): Reconciliation<SlideOperation>[] {
  const baseRuns = runOwners(base);
  return operations.map((operation): Reconciliation<SlideOperation> => {
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
    let value: unknown;
    switch (operation.type) {
      case "setFrame":
        value = current?.frame;
        break;
      case "setChartData":
        value = current?.chart?.data;
        break;
      case "setChartLegend":
        value = current?.chart?.legendPosition;
        break;
    }
    if (!current || value === undefined)
      return { operation, status: "missing" };
    if (same(value, proposedValue(operation)))
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

export function withCurrent(
  operation: SlideOperation,
  current: unknown,
): SlideOperation {
  return operation.type === "deleteElement"
    ? operation
    : ({ ...operation, expect: current } as SlideOperation);
}
