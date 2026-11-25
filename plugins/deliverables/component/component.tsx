import { useLayoutEffect, useMemo, useRef, useState } from "react";
import { type ComponentContext, useTranslator } from "@flame/component-sdk";
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
import type { DocOperation } from "./docs-draft";
import { WorkbookStudio } from "./sheets";
import type { CellOperation } from "./sheets-draft";
import type { Draft, DraftUpdate } from "./live-draft";
import type { SlideOperation } from "./slides-draft";
import { useDeliverables, type DeliverablesState } from "./useDeliverables";
import type { DocumentRead } from "./types";
import { en } from "./locales/en";
import { zh } from "./locales/zh";
import { zhTW } from "./locales/zh-TW";
import { ja } from "./locales/ja";
import { ko } from "./locales/ko";
import { es } from "./locales/es";
import { fr } from "./locales/fr";
import { de } from "./locales/de";
import { Capabilities, inspectRevision, readCatalog } from "./capabilities";
import { Notices, PendingCommands } from "./notices";
import { Inspector } from "./inspector";

const catalogs = { en, zh, "zh-TW": zhTW, ja, ko, es, fr, de };

function Workspace({
  context: initialContext,
  locale,
}: {
  context: ComponentContext;
  locale: string;
}) {
  const [context] = useState(initialContext);
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
  const slideDrafts = useDrafts<SlideOperation>();
  const sheetDrafts = useDrafts<CellOperation>();
  const docDrafts = useDrafts<DocOperation>();
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
          draft={slideDrafts.get(studioDocument.document.id)}
          onDraft={slideDrafts.update(studioDocument.document.id)}
          dateTime={dateTime}
          t={t}
        />
      )}
      {studioDocument?.revision.format === "docx" && (
        <DocumentStudio
          key={studioDocument.document.id}
          state={state}
          document={studioDocument}
          draft={docDrafts.get(studioDocument.document.id)}
          onDraft={docDrafts.update(studioDocument.document.id)}
          dateTime={dateTime}
          t={t}
        />
      )}
      {studioDocument?.revision.format === "xlsx" && (
        <WorkbookStudio
          key={studioDocument.document.id}
          state={state}
          document={studioDocument}
          draft={sheetDrafts.get(studioDocument.document.id)}
          onDraft={sheetDrafts.update(studioDocument.document.id)}
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

// The workspace owns drafts by document, so reading another revision (which
// unmounts an editor while it loads) never discards unsubmitted intent.
function useDrafts<Operation>() {
  const [drafts, setDrafts] = useState<Record<string, Draft<Operation>>>({});
  return {
    get: (id: string) => drafts[id],
    update: (id: string) => (update: DraftUpdate<Operation>) =>
      setDrafts((all) => {
        const { [id]: current, ...rest } = all;
        const next = update(current);
        return next ? { ...rest, [id]: next } : rest;
      }),
  };
}

export function Deliverables({ context }: { context: ComponentContext }) {
  // Display wrappers do not retire the captured I/O or authored drafts.
  const { runtime, component, binding, track, signal, saveFile } = context;
  const instance = useMemo(
    () => crypto.randomUUID(),
    [runtime, component, binding, track, signal, saveFile],
  );
  return <Workspace key={instance} context={context} locale={context.locale} />;
}
