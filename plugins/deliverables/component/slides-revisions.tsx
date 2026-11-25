import { useMemo } from "react";
import { type Translate } from "@flame/component-sdk";
import { type ChartData, type Frame } from "../contracts/content";
import { type ElementView, type SlideLayout } from "../contracts/layouts";
import type { Draft } from "./live-draft";
import { Reconcile } from "./live";
import { reconcile, withCurrent, type SlideOperation } from "./slides-draft";
import { chartText, frameText, label } from "./slides-presentation";

export function describeSlideOperation(
  operation: SlideOperation,
  base: SlideLayout | undefined,
  t: Translate,
) {
  if (operation.type === "replaceText")
    return t("slides.op.replaceText", { text: operation.text.slice(0, 60) });
  const element = base?.slides
    .flatMap((slide) => slide.elements)
    .find((entry) => entry.id === operation.id);
  const name = element ? label(element, t) : operation.id;
  return t(`slides.op.${operation.type}`, { name });
}

function show(operation: SlideOperation, value: unknown, t: Translate) {
  if (operation.type === "setFrame") return frameText(value as Frame);
  if (operation.type === "setChartData") return chartText(value as ChartData);
  if (operation.type === "deleteElement") return label(value as ElementView, t);
  return String(value);
}

export function SlideReconcile({
  base,
  head,
  draft,
  onRebase,
  onDiscard,
  t,
}: {
  base: SlideLayout;
  head: { id: string; layout: SlideLayout };
  draft: Draft<SlideOperation>;
  onRebase(draft: Draft<SlideOperation>): void;
  onDiscard(): void;
  t: Translate;
}) {
  const results = useMemo(
    () => reconcile(base, head.layout, draft.operations),
    [base, head.layout, draft.operations],
  );
  return (
    <Reconcile
      results={results}
      head={head.id}
      describe={(operation) => describeSlideOperation(operation, base, t)}
      mine={(operation) =>
        operation.type === "deleteElement"
          ? t("slides.kind.deleted")
          : show(
              operation,
              operation.type === "replaceText"
                ? operation.text
                : operation.type === "setFrame"
                  ? operation.frame
                  : operation.data,
              t,
            )
      }
      theirs={(result) => show(result.operation, result.current, t)}
      withCurrent={withCurrent}
      onRebase={onRebase}
      onDiscard={onDiscard}
      t={t}
    />
  );
}

export function savedElement(
  layout: SlideLayout | undefined,
  id: string | undefined,
): ElementView | undefined {
  if (id === undefined) return undefined;
  return layout?.slides
    .flatMap((entry) => entry.elements)
    .find((entry) => entry.id === id);
}
