import { useState } from "react";
import { type Translate } from "@flame/component-sdk";
import {
  Button,
  Callout,
  Disclosure,
  Facts,
  FormActions,
  Metadata,
  PanelSection,
  RecordBody,
  RecordGroup,
  RecordSummary,
  SearchField,
  Select,
  Toolbar,
} from "@flame/ui";
import { type FormatCatalog, type Inspection } from "../contracts/content";
import { METHODS } from "../contracts/methods";
import { RevisionEditor } from "./revisionEditor";
import { formatTimestamp, type OpenForReading } from "./archive";
import { EvidencePanel } from "./evidence";
import { Findings } from "./renderedPages";
import { type DeliverablesState } from "./useDeliverables";
import type { DocumentRead } from "./types";

function StructuredContent({
  inspection,
  t,
}: {
  inspection: Inspection;
  t: Translate;
}) {
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(0);
  const query = inlineContent(search).toLocaleLowerCase();
  const rows = inspection.items.filter(
    (item) =>
      !query ||
      inlineContent(`${item.id} ${item.text}`)
        .toLocaleLowerCase()
        .includes(query),
  );
  const pageSize = 25;
  const shown = rows.slice(page * pageSize, (page + 1) * pageSize);
  return (
    <Disclosure title={t("content.title")} keepMounted>
      <Metadata>{t("content.help")}</Metadata>
      {inspection.truncated && (
        <Callout kind="note">{t("content.truncated")}</Callout>
      )}
      <SearchField
        aria-label={t("content.search")}
        clearLabel={t("search.clear")}
        value={search}
        onChange={(event) => {
          setSearch(event.target.value);
          setPage(0);
        }}
        onClear={() => {
          setSearch("");
          setPage(0);
        }}
      />
      <RecordGroup variant="list" hidden={!shown.length}>
        {shown.map((item) => {
          const preview = contentPreview(item.text);
          const identity = `${item.kind} · ${item.id}`;
          return (
            <Disclosure
              key={item.id}
              variant="row"
              title={
                <RecordSummary
                  title={preview || identity}
                  description={preview ? identity : undefined}
                />
              }
            >
              <RecordBody>{item.text}</RecordBody>
            </Disclosure>
          );
        })}
      </RecordGroup>
      {!shown.length && <Metadata>{t("content.empty")}</Metadata>}
      {(page > 0 || rows.length > (page + 1) * pageSize) && (
        <Toolbar label={t("content.paging")}>
          <Button
            disabled={page === 0}
            focusableWhenDisabled
            onClick={() => setPage(page - 1)}
          >
            {t("previous")}
          </Button>
          <Button
            disabled={rows.length <= (page + 1) * pageSize}
            focusableWhenDisabled
            onClick={() => setPage(page + 1)}
          >
            {t("next")}
          </Button>
        </Toolbar>
      )}
    </Disclosure>
  );
}

function contentPreview(value: string): string {
  const text = inlineContent(value);
  const segments = Array.from(
    new Intl.Segmenter(undefined, { granularity: "grapheme" }).segment(text),
  );
  const boundary = segments[160]?.index;
  return boundary === undefined ? text : `${text.slice(0, boundary)}…`;
}

function inlineContent(value: string): string {
  return value.replace(/\s+/gu, " ").trim();
}

function safeSource(uri: string): string | undefined {
  try {
    const url = new URL(uri);
    return ["https:", "http:"].includes(url.protocol) &&
      !url.username &&
      !url.password
      ? url.href
      : undefined;
  } catch {
    return;
  }
}

function ProvenanceDisclosure({
  provenance,
  t,
}: {
  provenance: DocumentRead["revision"]["provenance"];
  t: Translate;
}) {
  return (
    <Disclosure title={t("provenance.title")}>
      <Metadata>{t("provenance.help")}</Metadata>
      {provenance.sources.map((source, index) => (
        <RecordGroup key={index}>
          <RecordBody>{source.title}</RecordBody>
          {safeSource(source.uri) ? (
            <a href={safeSource(source.uri)} target="_blank" rel="noreferrer">
              {source.uri}
            </a>
          ) : (
            <RecordBody>{source.uri}</RecordBody>
          )}
          {source.note && <Metadata>{source.note}</Metadata>}
        </RecordGroup>
      ))}
      <Facts
        values={[
          [
            t("provenance.assumptions"),
            provenance.assumptions.join("\n") || t("notRecorded"),
          ],
          [t("provenance.units"), provenance.units || t("notRecorded")],
          [
            t("provenance.reproduction"),
            provenance.reproduction || t("notRecorded"),
          ],
        ]}
      />
    </Disclosure>
  );
}

type Removal = { id: string; revision: string };

// Removal confirms against the head it was offered for; a newer head cancels it.
function RemovalDisclosure({
  state,
  document,
  current,
  removal,
  setRemoval,
  t,
}: {
  state: DeliverablesState;
  document: DocumentRead;
  current: boolean;
  removal: Removal | undefined;
  setRemoval: (removal: Removal | undefined) => void;
  t: Translate;
}) {
  return (
    <Disclosure title={t("remove.title")}>
      <Metadata>{t("remove.help")}</Metadata>
      {removal ? (
        <RecordGroup>
          <Callout kind="warning">
            {t("remove.confirmation", { title: document.document.title })}
          </Callout>
          <FormActions label={t("remove.actions")}>
            <Button
              disabled={state.blocked}
              onClick={() => setRemoval(undefined)}
            >
              {t("cancel")}
            </Button>
            <Button
              disabled={
                state.blocked || document.document.head !== removal.revision
              }
              onClick={() => {
                const captured = removal;
                void state.run(
                  METHODS.remove,
                  { id: captured.id, expectedRevision: captured.revision },
                  () => state.clearSelection(captured),
                );
              }}
            >
              {t("remove.confirm")}
            </Button>
          </FormActions>
        </RecordGroup>
      ) : (
        <Button
          disabled={state.blocked || !current}
          onClick={() =>
            setRemoval({
              id: document.document.id,
              revision: document.revision.id,
            })
          }
        >
          {t("remove.action")}
        </Button>
      )}
    </Disclosure>
  );
}

export function Inspector({
  state,
  document,
  inspection,
  catalog,
  dateTime,
  onOpen,
  t,
}: {
  state: DeliverablesState;
  document: DocumentRead;
  inspection: { value?: Inspection; error?: string };
  catalog?: FormatCatalog;
  dateTime: Intl.DateTimeFormat;
  onOpen: OpenForReading;
  t: Translate;
}) {
  const [removal, setRemoval] = useState<Removal>();
  const [csvSheet, setCSVSheet] = useState<string>();
  const current = document.document.head === document.revision.id;
  const provenance = document.revision.provenance;
  const sheets =
    inspection.value?.items.filter((item) => item.kind === "sheet") ?? [];
  const sheetPart = csvSheet ?? sheets[0]?.id;
  return (
    <PanelSection
      title={document.document.title}
      description={t(current ? "revision.current" : "revision.historical")}
    >
      <Facts
        values={[
          [t("author.format"), t(`format.${document.revision.format}`)],
          [
            t("revision.created"),
            formatTimestamp(document.revision.createdAt, dateTime),
          ],
          [
            t("revision.operation"),
            t(`operation.${document.revision.operation}`),
          ],
          [t("file.size"), String(document.revision.size)],
        ]}
      />
      <Toolbar label={t("export.actions")}>
        {(["native", "pdf", "bundle"] as const).map((kind) => (
          <Button
            key={kind}
            disabled={
              state.blocked ||
              !!state.transfer ||
              (kind === "pdf" &&
                !document.assessments.some((entry) => entry.kind === "render"))
            }
            onClick={() => {
              const captured = {
                id: document.document.id,
                revision: document.revision.id,
                kind,
              };
              void state.run(METHODS.export, captured).then((asset) => {
                if (asset) void state.save(asset);
              });
            }}
          >
            {t(`export.${kind}`)}
          </Button>
        ))}
        {document.revision.format === "xlsx" && (
          <>
            <Select
              label={t("author.sheet")}
              value={sheetPart ?? null}
              options={sheets.map((sheet) => ({
                value: sheet.id,
                label: sheet.text,
              }))}
              onValueChange={setCSVSheet}
            />
            <Button
              disabled={state.blocked || !!state.transfer || !sheetPart}
              onClick={() => {
                const captured = {
                  id: document.document.id,
                  revision: document.revision.id,
                  kind: "csv" as const,
                  sheetPart,
                };
                void state.run(METHODS.export, captured).then((asset) => {
                  if (asset) void state.save(asset);
                });
              }}
            >
              CSV
            </Button>
          </>
        )}
        {!current && (
          <Button
            onClick={(event) =>
              onOpen(
                { id: document.document.id, revision: document.document.head },
                event.currentTarget,
              )
            }
          >
            {t("revision.openHead")}
          </Button>
        )}
      </Toolbar>
      <Metadata>{t("export.help")}</Metadata>
      {inspection.error && <Callout kind="warning">{inspection.error}</Callout>}
      {inspection.value && (
        <>
          <Disclosure title={t("checks.title")} defaultOpen>
            <Findings checks={inspection.value.checks} t={t} />
          </Disclosure>
          <StructuredContent inspection={inspection.value} t={t} />
          <RevisionEditor
            state={state}
            document={document}
            inspection={inspection.value}
            catalog={catalog}
            t={t}
          />
        </>
      )}
      <EvidencePanel state={state} document={document} t={t} />
      <ProvenanceDisclosure provenance={provenance} t={t} />
      <RemovalDisclosure
        state={state}
        document={document}
        current={current}
        removal={removal}
        setRemoval={setRemoval}
        t={t}
      />
    </PanelSection>
  );
}
