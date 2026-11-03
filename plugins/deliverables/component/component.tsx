import { useLayoutEffect, useMemo, useRef, useState } from "react";
import {
  failureText,
  type ComponentContext,
  type Translate,
  useTranslator,
} from "@flame/component-sdk";
import {
  Button,
  Callout,
  Disclosure,
  Facts,
  FormActions,
  Metadata,
  Panel,
  PanelHeader,
  PanelSection,
  RecordBody,
  RecordGroup,
  RecordSummary,
  SearchField,
  Select,
  Toolbar,
} from "@flame/ui";
import {
  formatCatalogSchema,
  inspectionSchema,
  type FormatCatalog,
  type Inspection,
} from "../contracts/content";
import { engineCatalogSchema } from "../contracts/evidence";
import { METHODS } from "../contracts/methods";
import { CreateForm, RevisionEditor, Source } from "./authoring";
import {
  ArchiveList,
  RevisionHistory,
  ArchiveReadStatus,
  formatTimestamp,
  type OpenForReading,
} from "./archive";
import { ImportForm } from "./importing";
import { EvidencePanel, Findings } from "./evidence";
import {
  useDeliverables,
  type CommandMethod,
  type DeliverablesState,
} from "./useDeliverables";
import type { DocumentRead } from "./types";
import { en } from "./locales/en";
import { zh } from "./locales/zh";
import { zhTW } from "./locales/zh-TW";
import { ja } from "./locales/ja";
import { ko } from "./locales/ko";
import { es } from "./locales/es";
import { fr } from "./locales/fr";
import { de } from "./locales/de";

const catalogs = { en, zh, "zh-TW": zhTW, ja, ko, es, fr, de };

function commandLabel(method: CommandMethod, t: Translate): string {
  switch (method) {
    case METHODS.create:
      return t("author.create");
    case METHODS.edit:
      return t("edit.apply");
    case METHODS.upload:
    case METHODS.append:
    case METHODS.import:
      return t("import.submit");
    case METHODS.discard:
      return t("import.discard");
    case METHODS.render:
      return t("preview.render");
    case METHODS.recalculate:
      return t("preview.recalculate");
    case METHODS.review:
      return t("review.submit");
    case METHODS.export:
      return t("export.prepare");
    case METHODS.remove:
      return t("remove.action");
  }
}

function PendingCommands({
  state,
  t,
}: {
  state: DeliverablesState;
  t: Translate;
}) {
  if (!state.commands.length) return null;
  return (
    <PanelSection title={t("command.title")} description={t("command.help")}>
      {state.commands.map((command) => (
        <RecordGroup key={command.id}>
          <Metadata role="status">
            {commandLabel(command.method, t)} ·{" "}
            {t(
              command.status === "running"
                ? "command.running"
                : "command.unknown",
            )}
          </Metadata>
          {command.error && <Callout kind="warning">{command.error}</Callout>}
          <Toolbar label={t("command.actions")}>
            <Button
              disabled={command.status !== "unknown" || !command.recover}
              onClick={() => state.recover(command)}
            >
              {t("command.recover")}
            </Button>
            <Button
              disabled={command.status !== "unknown" || !command.retire}
              onClick={() => {
                void state.retire(command);
              }}
            >
              {t("command.retire")}
            </Button>
          </Toolbar>
          <Source label={t("command.identity")} value={command.id} />
        </RecordGroup>
      ))}
    </PanelSection>
  );
}

function Notices({
  state,
  onOpen,
  t,
}: {
  state: DeliverablesState;
  onOpen: OpenForReading;
  t: Translate;
}) {
  const notice = state.notice;
  return (
    <RecordGroup aria-live="polite">
      {notice?.kind === "error" && (
        <Callout kind="warning">{notice.text}</Callout>
      )}
      {notice?.kind === "save" && (
        <Metadata>
          {t(`save.${notice.result}`, { filename: notice.filename })}
        </Metadata>
      )}
      {notice?.kind === "command" && (
        <>
          <Metadata>
            {t("command.complete", { action: commandLabel(notice.method, t) })}
          </Metadata>
          {notice.asset && (
            <Button
              disabled={!!state.transfer}
              onClick={() => {
                if (notice.asset) void state.save(notice.asset);
              }}
            >
              {t("export.savePrepared")}
            </Button>
          )}
          {notice.revision && (
            <Button
              onClick={(event) => {
                if (notice.revision)
                  onOpen(
                    {
                      id: notice.revision.documentId,
                      revision: notice.revision.id,
                    },
                    event.currentTarget,
                  );
              }}
            >
              {t("command.openRevision")}
            </Button>
          )}
        </>
      )}
      {state.transfer && (
        <Toolbar label={t("transfer.title")}>
          <Metadata>
            {state.transfer.filename} ·{" "}
            {t("transfer.progress", {
              received: state.transfer.received,
              total: state.transfer.total,
            })}
          </Metadata>
          <Button
            disabled={state.transfer.received >= state.transfer.total}
            onClick={state.cancelTransfer}
          >
            {t("transfer.cancel")}
          </Button>
        </Toolbar>
      )}
    </RecordGroup>
  );
}

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

function Inspector({
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
  const [removal, setRemoval] = useState<{ id: string; revision: string }>();
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
    </PanelSection>
  );
}

function readCatalog(
  value: NonNullable<DeliverablesState["catalog"]>["value"],
) {
  if (!value) return;
  try {
    return {
      formats: formatCatalogSchema.parse(JSON.parse(value.formatsJSON)),
      engines: engineCatalogSchema.parse(JSON.parse(value.enginesJSON)),
    };
  } catch (error) {
    return { error: failureText(error) };
  }
}

function inspectRevision(document: DocumentRead | undefined): {
  value?: Inspection;
  error?: string;
} {
  if (!document) return {};
  try {
    const value = inspectionSchema.parse(JSON.parse(document.inspectionJSON));
    if (value.format !== document.revision.format)
      throw new Error(
        "deliverables: inspection format differs from its native revision",
      );
    return { value };
  } catch (error) {
    return { error: failureText(error) };
  }
}

function Capabilities({
  catalog,
  limits,
  t,
}: {
  catalog: ReturnType<typeof readCatalog>;
  limits?: string;
  t: Translate;
}) {
  return (
    <Disclosure title={t("capabilities.title")}>
      {catalog?.engines && (
        <>
          <Facts
            values={[
              [t("preview.platform"), catalog.engines.platform],
              [
                t("capabilities.configured"),
                t(catalog.engines.configured ? "yes" : "no"),
              ],
            ]}
          />
          {!catalog.engines.configured && (
            <Callout kind="note">{t("capabilities.setup")}</Callout>
          )}
          <Source
            label={t("capabilities.engines")}
            value={JSON.stringify(catalog.engines.configuration, null, 2)}
          />
          {catalog.engines.limitations.map((limitation) => (
            <Metadata key={limitation}>{limitation}</Metadata>
          ))}
        </>
      )}
      {catalog?.formats?.formats.map((entry) => (
        <Disclosure key={entry.format} title={t(`format.${entry.format}`)}>
          {entry.limitations.map((limitation) => (
            <Metadata key={limitation}>{limitation}</Metadata>
          ))}
          {entry.createSchemaJSON && (
            <Source label={t("author.schema")} value={entry.createSchemaJSON} />
          )}
          <Source label={t("edit.schema")} value={entry.editSchemaJSON} />
        </Disclosure>
      ))}
      {limits !== undefined && (
        <Source label={t("capabilities.limits")} value={limits} />
      )}
    </Disclosure>
  );
}

function Workspace({ context }: { context: ComponentContext }) {
  const t = useTranslator(catalogs, context.locale);
  const dateTime = useMemo(
    () =>
      new Intl.DateTimeFormat(context.locale, {
        dateStyle: "medium",
        timeStyle: "medium",
      }),
    [context.locale],
  );
  const state = useDeliverables(context);
  const historyEntry = useRef<HTMLElement>(null);
  // The pending page may lack enough scroll range to reveal the reader fully.
  const openingFocus = useRef<HTMLElement | null>(null);
  const openForReading: OpenForReading = (value, origin) => {
    openingFocus.current = null;
    if (state.signal.aborted) return;
    const ownedFocus = origin.ownerDocument.activeElement === origin;
    const settled =
      state.selection?.id === value.id &&
      state.selection.revision === value.revision &&
      state.read &&
      !state.read.loading;
    state.open(value);
    if (!ownedFocus) return;
    const entry = historyEntry.current;
    openingFocus.current = settled ? null : (entry ?? origin);
    entry?.focus({ preventScroll: true });
    if (settled) entry?.scrollIntoView({ block: "start", inline: "nearest" });
  };
  useLayoutEffect(() => {
    const owner = openingFocus.current;
    if (!owner) return;
    if (state.signal.aborted || owner.ownerDocument.activeElement !== owner) {
      openingFocus.current = null;
      return;
    }
    const entry = historyEntry.current;
    if (!entry) return;
    if (owner !== entry) {
      entry.focus({ preventScroll: true });
      openingFocus.current = entry;
    }
    if (state.read && !state.read.loading) {
      openingFocus.current = null;
      entry.scrollIntoView({ block: "start", inline: "nearest" });
    }
  }, [state.selection, state.read, state.signal]);
  const catalog = useMemo(
    () => readCatalog(state.catalog?.value),
    [state.catalog?.value],
  );
  const document = state.read?.value;
  const inspection = useMemo(
    () => inspectRevision(document),
    [document?.inspectionJSON, document?.revision.format],
  );
  const refreshing = state.list?.loading || state.catalog?.loading;
  const catalogError = state.catalog?.error ?? catalog?.error;
  return (
    <Panel
      label={t("panel.title")}
      header={<PanelHeader title={t("panel.title")} />}
      controls={
        <Toolbar label={t("panel.controls")}>
          <Button
            pending={refreshing}
            aria-busy={refreshing || undefined}
            onClick={() => {
              void state.refresh();
            }}
          >
            {t("refresh")}
          </Button>
        </Toolbar>
      }
    >
      <Notices state={state} onOpen={openForReading} t={t} />
      <PendingCommands state={state} t={t} />
      {catalogError && <Callout kind="warning">{catalogError}</Callout>}
      <Disclosure title={t("author.new")} keepMounted>
        <CreateForm state={state} catalog={catalog?.formats} t={t} />
      </Disclosure>
      <Disclosure title={t("import.open")} keepMounted>
        <ImportForm state={state} t={t} />
      </Disclosure>
      <ArchiveList
        state={state}
        dateTime={dateTime}
        onOpen={openForReading}
        t={t}
      />
      <RevisionHistory
        state={state}
        dateTime={dateTime}
        t={t}
        entry={historyEntry}
        inspection={inspection.value}
      />
      <ArchiveReadStatus read={state.read} t={t} />
      {document && (
        <Inspector
          key={document.revision.id}
          state={state}
          document={document}
          inspection={inspection}
          catalog={catalog?.formats}
          dateTime={dateTime}
          onOpen={openForReading}
          t={t}
        />
      )}
      <Capabilities
        catalog={catalog}
        limits={state.catalog?.value?.limitsJSON}
        t={t}
      />
    </Panel>
  );
}

export function Deliverables({ context }: { context: ComponentContext }) {
  // A new captured host context retires all local drafts, reads, and recovery observers.
  const instance = useMemo(() => crypto.randomUUID(), [context]);
  return <Workspace key={instance} context={context} />;
}
