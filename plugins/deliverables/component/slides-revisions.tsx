import { useMemo, useState } from "react";
import { type Translate } from "@flame/component-sdk";
import {
  Badge,
  Button,
  Callout,
  Facts,
  FormActions,
  Metadata,
  RecordGroup,
  Segmented,
} from "@flame/ui";
import {
  type ChartData,
  type ElementView,
  type Frame,
  type SlideLayout,
} from "../contracts/content";
import {
  rebase,
  reconcile,
  type Draft,
  type DraftOperation,
  type Reconciliation,
} from "./slides-draft";
import { type LayoutRead, inches, label } from "./slides-presentation";

function describe(
  operation: DraftOperation,
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

function Reconcile({
  base,
  head,
  draft,
  onRebase,
  onDiscard,
  t,
}: {
  base: SlideLayout;
  head: { id: string; layout: SlideLayout };
  draft: Draft;
  onRebase(draft: Draft): void;
  onDiscard(): void;
  t: Translate;
}) {
  const results = useMemo(
    () => reconcile(base, head.layout, draft.operations),
    [base, head.layout, draft.operations],
  );
  const [mine, setMine] = useState<ReadonlySet<number>>(new Set());
  const show = (operation: DraftOperation, value: unknown): string => {
    if (operation.type === "setFrame") {
      const frame = value as Frame;
      return `${inches(frame.x)}, ${inches(frame.y)} · ${inches(frame.width)} × ${inches(frame.height)} in`;
    }
    if (operation.type === "setChartData") {
      const data = value as ChartData;
      return data.series
        .map(
          (series) =>
            `${series.name}: ${data.categories
              .map(
                (category, point) =>
                  `${category} ${series.values[point] ?? "–"}`,
              )
              .join(" · ")}`,
        )
        .join("; ");
    }
    if (operation.type === "deleteElement")
      return label(value as ElementView, t);
    return String(value);
  };
  return (
    <RecordGroup>
      <Metadata>{t("slides.reconcile.help")}</Metadata>
      {results.map((result: Reconciliation, index) => (
        <RecordGroup key={index}>
          <Metadata>
            {describe(result.operation, base, t)} ·{" "}
            <Badge>{t(`slides.status.${result.status}`)}</Badge>
          </Metadata>
          {result.status === "conflict" && (
            <>
              <Facts
                values={[
                  [
                    t("slides.mine"),
                    result.operation.type === "deleteElement"
                      ? t("slides.kind.deleted")
                      : show(
                          result.operation,
                          "text" in result.operation
                            ? result.operation.text
                            : "frame" in result.operation
                              ? result.operation.frame
                              : result.operation.data,
                        ),
                  ],
                  [t("slides.theirs"), show(result.operation, result.current)],
                ]}
              />
              <Segmented
                ariaLabel={t("slides.reconcile.choice")}
                value={mine.has(index) ? "mine" : "theirs"}
                options={[
                  { value: "theirs", label: t("slides.keepCurrent") },
                  { value: "mine", label: t("slides.keepMine") },
                ]}
                onChange={(value) => {
                  const next = new Set(mine);
                  if (value === "mine") next.add(index);
                  else next.delete(index);
                  setMine(next);
                }}
              />
            </>
          )}
        </RecordGroup>
      ))}
      <FormActions label={t("slides.reconcile.actions")}>
        <Button onClick={onDiscard}>{t("slides.discard")}</Button>
        <Button
          variant="primary"
          onClick={() => onRebase(rebase(head.id, results, mine))}
        >
          {t("slides.rebase")}
        </Button>
      </FormActions>
    </RecordGroup>
  );
}

export type Handoff = "handed" | "stale" | "copied" | "failed";

export function savedElement(
  layout: SlideLayout | undefined,
  id: string | undefined,
): ElementView | undefined {
  if (id === undefined) return undefined;
  return layout?.slides
    .flatMap((entry) => entry.elements)
    .find((entry) => entry.id === id);
}

export function RevisionNotices({
  t,
  viewed,
  head,
  draft,
  editing,
  pending,
  read,
  baseRead,
  onOpen,
  onRebase,
  onDiscard,
  onRetry,
}: {
  t: Translate;
  viewed: string;
  head: string;
  draft: Draft | undefined;
  editing: boolean;
  pending: string | undefined;
  read: LayoutRead | undefined;
  baseRead: LayoutRead | undefined;
  onOpen(revision: string): void;
  onRebase(draft: Draft): void;
  onDiscard(): void;
  onRetry(): void;
}) {
  return (
    <>
      {viewed !== head && !draft && (
        <Callout kind="note">{t("slides.historical")}</Callout>
      )}
      {editing && head !== viewed && (
        <Callout kind="warning">
          {t("slides.newer")}{" "}
          <Button onClick={() => onOpen(head)}>{t("slides.compare")}</Button>
        </Callout>
      )}
      {pending && draft && (
        <RecordGroup>
          {viewed !== head ? (
            <Callout kind="note">
              {t("slides.draftElsewhere")}{" "}
              <Button onClick={() => onOpen(pending)}>
                {t("slides.openDraft")}
              </Button>
            </Callout>
          ) : baseRead?.value && read?.value ? (
            <Reconcile
              base={baseRead.value}
              head={{ id: head, layout: read.value }}
              draft={draft}
              onRebase={onRebase}
              onDiscard={onDiscard}
              t={t}
            />
          ) : (
            <Metadata role="status">{t("slides.loading")}</Metadata>
          )}
        </RecordGroup>
      )}
      {(read?.error || baseRead?.error) && (
        <Callout kind="warning">
          {read?.error ?? baseRead?.error}{" "}
          <Button onClick={onRetry}>{t("slides.retry")}</Button>
        </Callout>
      )}
      {!read && <Metadata role="status">{t("slides.loading")}</Metadata>}
    </>
  );
}
