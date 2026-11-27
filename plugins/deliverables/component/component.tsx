import {
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  useSyncExternalStore,
} from "react";
import {
  type ComponentContext,
  type PluginComponentProps,
  useTranslator,
} from "@flame/component-sdk";
import {
  Button,
  Callout,
  Disclosure,
  Panel,
  PanelHeader,
  Toolbar,
} from "@flame/ui";
import { CreateForm } from "./authoring";
import {
  ArchiveList,
  RevisionHistory,
  ArchiveReadStatus,
  type OpenForReading,
} from "./archive";
import { ImportForm } from "./importing";
import { SlideStudio } from "./slides";
import { DocumentStudio } from "./docs";
import { WorkbookStudio } from "./sheets";
import { useDeliverables, type DeliverablesState } from "./useDeliverables";
import type { DocumentRead } from "./types";
import { catalogs } from "./catalogs";
import { Capabilities, inspectRevision, readCatalog } from "./capabilities";
import { Notices, PendingCommands } from "./notices";
import { Inspector } from "./inspector";
import type { DraftFormat, DraftOperation } from "../contracts/content";
import { DraftStore } from "./draft-storage";
import type { DraftUpdate } from "./live-draft";

export { DeliverableResult } from "./tool-result";

function Workspace({ context, locale }: PluginComponentProps) {
  const t = useTranslator(catalogs, locale);
  const dateTime = useMemo(
    () =>
      new Intl.DateTimeFormat(locale, {
        dateStyle: "medium",
        timeStyle: "medium",
      }),
    [locale],
  );
  const state = useDeliverables(context);
  const bodyViewport = useRef<HTMLDivElement>(null);
  const archiveSearchField = useRef<HTMLInputElement>(null);
  const archiveOrigin = useRef<{
    documentId: string;
    element: HTMLElement;
    scrollTop: number;
  }>(null);
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
  const openFromArchive: OpenForReading = (value, origin) => {
    const viewport = bodyViewport.current;
    if (viewport && !state.signal.aborted) {
      archiveOrigin.current = {
        documentId: value.id,
        element: origin,
        scrollTop: viewport.scrollTop,
      };
    }
    openForReading(value, origin);
  };
  const returnToArchive = () => {
    openingFocus.current = null;
    const viewport = bodyViewport.current;
    const origin = archiveOrigin.current;
    if (
      viewport &&
      origin &&
      origin.documentId === state.selection?.id &&
      viewport.contains(origin.element)
    ) {
      viewport.scrollTop = origin.scrollTop;
      origin.element.focus({ preventScroll: true });
      if (origin.element.ownerDocument.activeElement === origin.element) {
        origin.element.scrollIntoView({ block: "nearest", inline: "nearest" });
        return;
      }
    }
    archiveSearchField.current?.focus();
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
  const studioDocument = useStudioDocument(state);
  const focus = state.selection?.id === studioDocument?.document.id
    ? state.selection
    : undefined;
  const slideDraft = useDocumentDraft(context, "pptx", studioDocument);
  const sheetDraft = useDocumentDraft(context, "xlsx", studioDocument);
  const docDraft = useDocumentDraft(context, "docx", studioDocument);
  const inspection = useMemo(
    () => inspectRevision(document),
    [document?.inspectionJSON, document?.revision.format],
  );
  const refreshing = state.list?.loading || state.catalog?.loading;
  const catalogError = state.catalog?.error ?? catalog?.error;
  return (
    <Panel
      bodyViewportRef={bodyViewport}
      label={t("panel.title")}
      header={<PanelHeader title={t("panel.title")} />}
      controls={
        <Toolbar label={t("panel.controls")}>
          {state.selection && (
            <Button onClick={returnToArchive}>{t("list.back")}</Button>
          )}
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
      {state.linkError && (
        <Callout kind="warning">
          {t("link.unavailable")} {state.linkError}
        </Callout>
      )}
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
        onOpen={openFromArchive}
        searchFieldRef={archiveSearchField}
        t={t}
      />
      <RevisionHistory
        state={state}
        dateTime={dateTime}
        t={t}
        entry={historyEntry}
        inspection={inspection.value}
      />
      <ArchiveReadStatus
        read={studioDocument === document ? state.read : undefined}
        t={t}
      />
      {studioDocument?.revision.format === "pptx" && (
        <SlideStudio
          key={studioDocument.document.id}
          state={state}
          document={studioDocument}
          {...slideDraft}
          link={focus}
          dateTime={dateTime}
          t={t}
        />
      )}
      {studioDocument?.revision.format === "docx" && (
        <DocumentStudio
          key={studioDocument.document.id}
          state={state}
          document={studioDocument}
          {...docDraft}
          link={focus}
          dateTime={dateTime}
          t={t}
        />
      )}
      {studioDocument?.revision.format === "xlsx" && (
        <WorkbookStudio
          key={studioDocument.document.id}
          state={state}
          document={studioDocument}
          {...sheetDraft}
          link={focus}
          dateTime={dateTime}
          t={t}
        />
      )}
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

// An editor keeps the revision it shows while another revision of the same
// document is read, so it stays mounted and keeps its place.
function useStudioDocument(state: DeliverablesState): DocumentRead | undefined {
  const document = state.read?.value;
  const [last, setLast] = useState(document);
  if (document && document !== last) setLast(document);
  if (document) return document;
  return !state.read?.error && last?.document.id === state.selection?.id
    ? last
    : undefined;
}

function useDocumentDraft<Format extends DraftFormat>(
  context: ComponentContext,
  format: Format,
  document: DocumentRead | undefined,
) {
  const [store] = useState(
    () => new DraftStore(context.component.installationId, format),
  );
  const id =
    document?.revision.format === format ? document.document.id : undefined;
  const draft = useSyncExternalStore(store.subscribe, () =>
    id === undefined ? undefined : store.get(id),
  );
  return {
    draft,
    onDraft: (update: DraftUpdate<DraftOperation<Format>>) => {
      if (id !== undefined) store.update(id, update);
    },
  };
}

export function Deliverables({ context, locale }: PluginComponentProps) {
  const instance = useMemo(() => crypto.randomUUID(), [context.signal]);
  return <Workspace key={instance} context={context} locale={locale} />;
}
