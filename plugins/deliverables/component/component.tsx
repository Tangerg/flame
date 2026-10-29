import { useMemo, useState } from "react";
import {
  createTranslator,
  failureText,
  type ComponentContext,
  type Translate,
} from "@flame/component-sdk";
import {
  Badge,
  Button,
  Callout,
  Disclosure,
  EmptyState,
  Facts,
  FormActions,
  Metadata,
  Panel,
  PanelHeader,
  PanelSection,
  RecordBody,
  RecordButton,
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

function Notices({ state, t }: { state: DeliverablesState; t: Translate }) {
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
              onClick={() => {
                if (notice.revision)
                  state.open({
                    id: notice.revision.documentId,
                    revision: notice.revision.id,
                  });
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
  const rows = inspection.items.filter(
    (item) =>
      !search ||
      `${item.id} ${item.text}`
        .toLocaleLowerCase()
        .includes(search.toLocaleLowerCase()),
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
        {shown.map((item) => (
          <Disclosure
            key={item.id}
            variant="row"
            title={`${item.kind} · ${item.id}`}
          >
            <RecordBody>{item.text}</RecordBody>
          </Disclosure>
        ))}
      </RecordGroup>
      {!shown.length && <Metadata>{t("content.empty")}</Metadata>}
      {(page > 0 || rows.length > (page + 1) * pageSize) && (
        <Toolbar label={t("content.paging")}>
          <Button disabled={page === 0} onClick={() => setPage(page - 1)}>
            {t("previous")}
          </Button>
          <Button
            disabled={rows.length <= (page + 1) * pageSize}
            onClick={() => setPage(page + 1)}
          >
            {t("next")}
          </Button>
        </Toolbar>
      )}
    </Disclosure>
  );
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
  catalog,
  t,
}: {
  state: DeliverablesState;
  document: DocumentRead;
  catalog?: FormatCatalog;
  t: Translate;
}) {
  const [removal, setRemoval] = useState<{ id: string; revision: string }>();
  const [csvSheet, setCSVSheet] = useState<string>();
  const inspection = useMemo(() => {
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
  }, [document.inspectionJSON, document.revision.format]);
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
          [t("revision.created"), document.revision.createdAt],
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
            onClick={() =>
              state.open({
                id: document.document.id,
                revision: document.document.head,
              })
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
      <Disclosure title={t("revision.details")}>
        <Facts
          values={[
            [t("document.id"), document.document.id],
            [t("revision.id"), document.revision.id],
            [
              t("revision.parent"),
              document.revision.parent ?? t("revision.first"),
            ],
            [t("file.hash"), document.revision.sha256],
          ]}
        />
        {inspection.value && (
          <Facts
            values={Object.entries(inspection.value.facts).map(
              ([key, value]) => [key, String(value)],
            )}
          />
        )}
        <Source label={t("content.original")} value={document.inspectionJSON} />
        {document.processingJSON && (
          <Source
            label={t("revision.processing")}
            value={document.processingJSON}
          />
        )}
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

function Workspace({ context }: { context: ComponentContext }) {
  const t = useMemo(
    () => createTranslator(catalogs, context.locale),
    [context.locale],
  );
  const state = useDeliverables(context);
  const [createOpen, setCreateOpen] = useState(false);
  const [importOpen, setImportOpen] = useState(false);
  const [search, setSearch] = useState("");
  const [format, setFormat] = useState("all");
  const catalog = useMemo(() => {
    if (!state.catalog?.value) return;
    try {
      return {
        formats: formatCatalogSchema.parse(
          JSON.parse(state.catalog.value.formatsJSON),
        ),
        engines: engineCatalogSchema.parse(
          JSON.parse(state.catalog.value.enginesJSON),
        ),
      };
    } catch (error) {
      return { error: failureText(error) };
    }
  }, [state.catalog?.value]);
  const entries =
    state.list?.value?.entries.filter(
      (entry) =>
        (format === "all" || entry.format === format) &&
        entry.title.toLocaleLowerCase().includes(search.toLocaleLowerCase()),
    ) ?? [];
  const document = state.read?.value;
  return (
    <Panel
      label={t("panel.title")}
      header={
        <PanelHeader
          title={t("panel.title")}
          description={t("panel.description")}
        />
      }
      controls={
        <Toolbar label={t("panel.controls")}>
          <Button
            variant="primary"
            onClick={() => setCreateOpen((open) => !open)}
          >
            {t("author.new")}
          </Button>
          <Button onClick={() => setImportOpen((open) => !open)}>
            {t("import.open")}
          </Button>
          <Button
            disabled={state.list?.loading || state.catalog?.loading}
            onClick={() => {
              void state.refresh();
            }}
          >
            {t("refresh")}
          </Button>
        </Toolbar>
      }
    >
      <Notices state={state} t={t} />
      <PendingCommands state={state} t={t} />
      {(state.catalog?.error || catalog?.error) && (
        <Callout kind="warning">
          {state.catalog?.error ?? catalog?.error}
        </Callout>
      )}
      <Disclosure
        title={t("author.new")}
        open={createOpen}
        onOpenChange={setCreateOpen}
        keepMounted
      >
        <CreateForm state={state} catalog={catalog?.formats} t={t} />
      </Disclosure>
      <Disclosure
        title={t("import.open")}
        open={importOpen}
        onOpenChange={setImportOpen}
        keepMounted
      >
        <ImportForm state={state} t={t} />
      </Disclosure>
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
              ...["docx", "xlsx", "pptx", "pdf"].map((value) => ({
                value,
                label: t(`format.${value}`),
              })),
            ]}
          />
        </Toolbar>
        {state.list?.loading && (
          <Metadata role="status">{t("reading")}</Metadata>
        )}
        {state.list?.error && (
          <Callout kind="warning">{state.list.error}</Callout>
        )}
        <RecordGroup variant="list" hidden={!entries.length}>
          {entries.map((entry) => (
            <RecordButton
              key={entry.id}
              aria-current={
                state.selection?.id === entry.id ? "true" : undefined
              }
              onClick={() => state.open({ id: entry.id, revision: entry.head })}
            >
              <RecordSummary
                title={entry.title}
                description={entry.createdAt}
                status={<Badge>{entry.format.toUpperCase()}</Badge>}
              />
            </RecordButton>
          ))}
        </RecordGroup>
        {state.list?.value && !entries.length && (
          <EmptyState title={t("list.empty")} sub={t("list.emptyHelp")} />
        )}
        {(state.listPrevious || state.list?.value?.next) && (
          <Toolbar label={t("list.paging")}>
            <Button
              disabled={!state.listPrevious}
              pending={state.list?.loading}
              aria-busy={state.list?.loading || undefined}
              focusableWhenDisabled
              onClick={() => {
                void state.page("list", "previous");
              }}
            >
              {t("previous")}
            </Button>
            <Button
              disabled={!state.list?.value?.next}
              pending={state.list?.loading}
              aria-busy={state.list?.loading || undefined}
              focusableWhenDisabled
              onClick={() => {
                void state.page("list", "next");
              }}
            >
              {t("next")}
            </Button>
          </Toolbar>
        )}
      </PanelSection>
      {state.selection && (
        <PanelSection title={t("history.title")}>
          {state.history?.loading && (
            <Metadata role="status">{t("reading")}</Metadata>
          )}
          {state.history?.error && (
            <Callout kind="warning">{state.history.error}</Callout>
          )}
          <Select
            label={t("history.choose")}
            value={state.selection.revision}
            onValueChange={(revision) => {
              if (state.selection)
                state.open({ id: state.selection.id, revision });
            }}
            options={(state.history?.value?.entries ?? []).map((revision) => ({
              value: revision.id,
              label: `${revision.createdAt} · ${t(`operation.${revision.operation}`)} · ${revision.id.slice(0, 12)}`,
            }))}
          />
          {(state.historyPrevious || state.history?.value?.next) && (
            <Toolbar label={t("history.paging")}>
              <Button
                disabled={!state.historyPrevious}
                pending={state.history?.loading}
                aria-busy={state.history?.loading || undefined}
                focusableWhenDisabled
                onClick={() => {
                  void state.page("history", "previous");
                }}
              >
                {t("previous")}
              </Button>
              <Button
                disabled={!state.history?.value?.next}
                pending={state.history?.loading}
                aria-busy={state.history?.loading || undefined}
                focusableWhenDisabled
                onClick={() => {
                  void state.page("history", "next");
                }}
              >
                {t("next")}
              </Button>
            </Toolbar>
          )}
        </PanelSection>
      )}
      {state.read?.loading && <Metadata role="status">{t("reading")}</Metadata>}
      {state.read?.error && (
        <Callout kind="warning">{state.read.error}</Callout>
      )}
      {document && (
        <Inspector
          key={document.revision.id}
          state={state}
          document={document}
          catalog={catalog?.formats}
          t={t}
        />
      )}
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
              <Source
                label={t("author.schema")}
                value={entry.createSchemaJSON}
              />
            )}
            <Source label={t("edit.schema")} value={entry.editSchemaJSON} />
          </Disclosure>
        ))}
        {state.catalog?.value && (
          <Source
            label={t("capabilities.limits")}
            value={state.catalog.value.limitsJSON}
          />
        )}
      </Disclosure>
    </Panel>
  );
}

export function Deliverables({ context }: { context: ComponentContext }) {
  // A new captured host context retires all local drafts, reads, and recovery observers.
  const instance = useMemo(() => crypto.randomUUID(), [context]);
  return <Workspace key={instance} context={context} />;
}
