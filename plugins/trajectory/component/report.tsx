import { useEffect, useMemo, useState } from "react";
import type { Ref } from "react";
import type { Translate } from "@flame/component-sdk";
import { Activity } from "@flame/icons/react";
import type { Page, TrajectoryEntry } from "@flame/runtime-client/wire";
import {
  Button,
  Disclosure,
  EmptyState,
  Facts,
  Metadata,
  RecordGroup,
  RecordMeta,
} from "@flame/ui";
import { type ExpandedRecord, Observation, title } from "./observation";
import { matches, needsAttention, recordKey, runId } from "./recordFacts";

export function TrajectoryReport({
  page,
  pageNumber,
  kind,
  run,
  search,
  stale,
  onClearFilters,
  ref,
  t,
  locale,
}: {
  page: Page<TrajectoryEntry>;
  pageNumber: number;
  kind: string;
  run: string;
  search: string;
  stale?: boolean;
  onClearFilters: () => void;
  ref?: Ref<HTMLParagraphElement>;
  t: Translate;
  locale: string;
}) {
  const rows = page.data;
  const [expanded, setExpanded] = useState<ReadonlyMap<string, ExpandedRecord>>(
    () => new Map(),
  );
  useEffect(() => {
    setExpanded((current) => {
      if (!current.size) return current;
      const keys = new Set(rows.map(recordKey));
      const retained = new Map([...current].filter(([key]) => keys.has(key)));
      return retained.size === current.size ? current : retained;
    });
  }, [rows]);

  function setRecordOpen(key: string, open: boolean) {
    setExpanded((current) => {
      if (current.has(key) === open) return current;
      const next = new Map(current);
      if (open)
        next.set(key, {
          evidenceOpen: false,
          sourcePosition: { current: null },
        });
      else next.delete(key);
      return next;
    });
  }
  function setEvidenceOpen(key: string, open: boolean) {
    setExpanded((current) => {
      const expansion = current.get(key);
      if (!expansion || expansion.evidenceOpen === open) return current;
      // A closing panel can detach after the next evidence reader has opened.
      return new Map(current).set(key, {
        evidenceOpen: open,
        sourcePosition: { current: null },
      });
    });
  }

  const query = search.toLocaleLowerCase();
  const matching = rows.filter((row) => {
    const category = row.type === "item" ? row.item.type : row.type;
    const matchesKind =
      kind === "all" ||
      kind === category ||
      (kind === "attention" && needsAttention(row)) ||
      (kind === "message" &&
        ["userMessage", "agentMessage", "reasoning"].includes(category));
    return (
      matchesKind &&
      (run === "all" || runId(row) === run) &&
      matches(row, query, t)
    );
  });
  const toolCalls = rows.filter(
    (row) => row.type === "item" && row.item.type === "toolCall",
  ).length;
  const attention = rows.filter(needsAttention).length;
  const clock = useMemo(
    () =>
      new Intl.DateTimeFormat(locale, {
        hour: "numeric",
        minute: "2-digit",
        second: "2-digit",
      }),
    [locale],
  );
  const occurred = useMemo(
    () =>
      new Intl.DateTimeFormat(locale, {
        dateStyle: "medium",
        timeStyle: "medium",
      }),
    [locale],
  );
  return (
    <>
      <Metadata ref={ref} tabIndex={-1} role="status">
        <RecordMeta>
          {(pageNumber > 1 || Boolean(page.nextCursor)) && (
            <span>{t("records.page", { page: pageNumber })}</span>
          )}
          <span>
            {matching.length === rows.length
              ? t("records.count", { count: rows.length })
              : t("records.matching", {
                  count: rows.length,
                  matching: matching.length,
                })}
          </span>
          <span>{t("toolCalls", { count: toolCalls })}</span>
          {attention > 0 && <span>{t("attention", { count: attention })}</span>}
          {stale && <span>{t("summary.stale")}</span>}
        </RecordMeta>
      </Metadata>
      <RecordGroup variant="list" id="records" aria-label={t("records.label")}>
        {matching.map((row) => {
          const key = recordKey(row);
          const expansion = expanded.get(key);
          return (
            <Disclosure
              variant="row"
              key={key}
              data-trajectory-record={key}
              data-trajectory-kind={
                row.type === "item" ? row.item.type : row.type
              }
              title={title(row, t, clock)}
              open={expansion !== undefined}
              onOpenChange={(open) => setRecordOpen(key, open)}
            >
              <Metadata>
                <time dateTime={row.occurredAt}>
                  {occurred.format(new Date(row.occurredAt))}
                </time>
              </Metadata>
              <Observation
                row={row}
                t={t}
                expansion={expansion}
                onEvidenceOpenChange={(open) => setEvidenceOpen(key, open)}
              />
            </Disclosure>
          );
        })}
        {!matching.length && (
          <EmptyState
            icon={Activity}
            size="compact"
            title={
              rows.length ? t("empty.filtered.title") : t("empty.none.title")
            }
            action={
              (kind !== "all" || run !== "all" || search !== "") && (
                <Button variant="outline" onClick={onClearFilters}>
                  {t("filters.clear")}
                </Button>
              )
            }
          />
        )}
      </RecordGroup>
      <Disclosure title={t("modelActivity.title")}>
        <Facts
          values={[
            [
              t("fact.logicalModelCalls"),
              rows.filter((row) => row.type === "model").length,
            ],
            [
              t("fact.modelEndpoints"),
              rows.filter((row) => row.type === "endpoint").length,
            ],
          ]}
        />
      </Disclosure>
      <Disclosure title={t("about.title")}>
        <Metadata>{t("about.note")}</Metadata>
      </Disclosure>
    </>
  );
}
