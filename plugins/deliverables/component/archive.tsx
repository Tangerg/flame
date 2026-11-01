import { useState, type Ref } from "react";
import type { Translate } from "@flame/component-sdk";
import {
  Badge,
  Button,
  Callout,
  Disclosure,
  EmptyState,
  Facts,
  Metadata,
  PanelSection,
  RecordButton,
  RecordGroup,
  RecordSummary,
  SearchField,
  Select,
  Toolbar,
} from "@flame/ui";
import { formatSchema, type Inspection } from "../contracts/content";
import { Source } from "./authoring";
import type { DeliverablesState } from "./useDeliverables";
import type { DocumentRead } from "./types";

type ArchiveProps = {
  state: DeliverablesState;
  dateTime: Intl.DateTimeFormat;
  t: Translate;
};
type Revision = DocumentRead["revision"];
type Selection = NonNullable<DeliverablesState["selection"]>;

export function formatTimestamp(
  value: string,
  format: Intl.DateTimeFormat,
): string {
  const date = new Date(value);
  // The wire also allows opaque text; interpret only the producer's canonical ISO form.
  return Number.isFinite(date.getTime()) && date.toISOString() === value
    ? format.format(date)
    : value;
}

export function ArchiveReadStatus({
  read,
  t,
}: {
  read?: { loading: boolean; error?: string };
  t: Translate;
}) {
  return (
    <>
      {read?.loading && <Metadata role="status">{t("reading")}</Metadata>}
      {read?.error && <Callout kind="warning">{read.error}</Callout>}
    </>
  );
}

function ArchivePaging({
  state,
  page,
  t,
}: {
  state: DeliverablesState;
  page: "list" | "history";
  t: Translate;
}) {
  const read = state[page];
  const previous = page === "list" ? state.listPrevious : state.historyPrevious;
  const next = read?.value?.next;
  if (!previous && !next) return null;
  const loading = read?.loading;
  return (
    <Toolbar label={t(page === "list" ? "list.paging" : "history.paging")}>
      <Button
        disabled={!previous}
        pending={loading}
        aria-busy={loading || undefined}
        focusableWhenDisabled
        onClick={() => {
          void state.page(page, "previous");
        }}
      >
        {t("previous")}
      </Button>
      <Button
        disabled={!next}
        pending={loading}
        aria-busy={loading || undefined}
        focusableWhenDisabled
        onClick={() => {
          void state.page(page, "next");
        }}
      >
        {t("next")}
      </Button>
    </Toolbar>
  );
}

export function ArchiveList({ state, dateTime, t }: ArchiveProps) {
  const [search, setSearch] = useState("");
  const [format, setFormat] = useState("all");
  const entries =
    state.list?.value?.entries.filter(
      (entry) =>
        (format === "all" || entry.format === format) &&
        entry.title.toLocaleLowerCase().includes(search.toLocaleLowerCase()),
    ) ?? [];
  return (
    <PanelSection title={t("list.title")}>
      <Toolbar label={t("list.filters")}>
        <SearchField
          aria-label={t("list.search")}
          value={search}
          clearLabel={t("search.clear")}
          onChange={(event) => setSearch(event.target.value)}
          onClear={() => setSearch("")}
        />
        <Select
          label={t("list.format")}
          value={format}
          onValueChange={setFormat}
          options={[
            { value: "all", label: t("list.all") },
            ...formatSchema.options.map((value) => ({
              value,
              label: t(`format.${value}`),
            })),
          ]}
        />
      </Toolbar>
      <ArchiveReadStatus read={state.list} t={t} />
      <RecordGroup variant="list" hidden={!entries.length}>
        {entries.map((entry) => (
          <RecordButton
            key={entry.id}
            aria-current={state.selection?.id === entry.id ? "true" : undefined}
            onClick={() => state.open({ id: entry.id, revision: entry.head })}
          >
            <RecordSummary
              title={entry.title}
              description={formatTimestamp(entry.createdAt, dateTime)}
              status={<Badge>{entry.format.toUpperCase()}</Badge>}
            />
          </RecordButton>
        ))}
      </RecordGroup>
      {state.list?.value && !entries.length && (
        <EmptyState title={t("list.empty")} sub={t("list.emptyHelp")} />
      )}
      <ArchivePaging state={state} page="list" t={t} />
    </PanelSection>
  );
}

function revisionChoices(
  revisions: readonly Revision[],
  selected: string,
  dateTime: Intl.DateTimeFormat,
  t: Translate,
) {
  const revisionOptions = revisions.map((revision) => ({
    value: revision.id,
    label: `${formatTimestamp(revision.createdAt, dateTime)} · ${t(`operation.${revision.operation}`)} · ${revision.id.slice(0, 12)}`,
  }));
  if (!revisionOptions.some((option) => option.value === selected))
    revisionOptions.unshift({
      value: selected,
      label: selected.slice(0, 12),
    });
  return revisionOptions;
}

function RevisionDetails({
  selection,
  created,
  revisionCreated,
  document,
  inspection,
  t,
}: {
  selection: Selection;
  created?: string;
  revisionCreated?: string;
  document?: DocumentRead;
  inspection?: Inspection;
  t: Translate;
}) {
  return (
    <Disclosure title={t("revision.details")}>
      <Facts
        values={[
          [t("document.id"), selection.id],
          [t("document.created"), created],
          [t("revision.id"), selection.revision],
          [t("revision.created"), revisionCreated],
        ]}
      />
      {document && (
        <>
          <Facts
            values={[
              [
                t("revision.parent"),
                document.revision.parent ?? t("revision.first"),
              ],
              [t("file.hash"), document.revision.sha256],
            ]}
          />
          {inspection && (
            <Facts
              values={Object.entries(inspection.facts).map(([key, value]) => [
                key,
                String(value),
              ])}
            />
          )}
          <Source
            label={t("content.original")}
            value={document.inspectionJSON}
          />
          {document.processingJSON && (
            <Source
              label={t("revision.processing")}
              value={document.processingJSON}
            />
          )}
        </>
      )}
    </Disclosure>
  );
}

export function RevisionHistory({
  state,
  dateTime,
  t,
  entry,
  inspection,
}: ArchiveProps & { entry: Ref<HTMLElement>; inspection?: Inspection }) {
  const selection = state.selection;
  if (!selection) return null;
  const document = state.read?.value;
  const history = state.history?.value?.entries ?? [];
  const revisions =
    document &&
    !history.some((revision) => revision.id === document.revision.id)
      ? [document.revision, ...history]
      : history;
  const selectedDocument =
    document?.document ??
    state.list?.value?.entries.find((entry) => entry.id === selection.id);
  const selectedRevision = revisions.find(
    (revision) => revision.id === selection.revision,
  );
  return (
    <PanelSection
      ref={entry}
      tabIndex={-1}
      aria-label={t("history.title")}
      title={t("history.title")}
    >
      <ArchiveReadStatus read={state.history} t={t} />
      <Select
        label={t("history.choose")}
        value={selection.revision}
        onValueChange={(revision) => state.open({ id: selection.id, revision })}
        options={revisionChoices(revisions, selection.revision, dateTime, t)}
      />
      <ArchivePaging state={state} page="history" t={t} />
      <RevisionDetails
        selection={selection}
        created={selectedDocument?.createdAt}
        revisionCreated={selectedRevision?.createdAt}
        document={document}
        inspection={inspection}
        t={t}
      />
    </PanelSection>
  );
}
