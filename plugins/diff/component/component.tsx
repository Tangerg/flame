import { summarizeWorkingTreeChanges } from "./workingTreeChanges";
import { useId, useLayoutEffect, useMemo, useRef, useState } from "react";
import {
  type ComponentContext,
  type WorkspaceChangesActionInput,
  useTranslator,
} from "@flame/component-sdk";
import { Copy } from "@flame/icons/react";
import {
  Button,
  CodeWrapButton,
  DiffFileHeader,
  DiffStat,
  FilterBar,
  IconButton,
  isImeKey,
  Panel,
  PanelHeader,
  SearchField,
  Select,
  useCopy,
} from "@flame/ui";
import { en } from "./locales/en";
import { zh } from "./locales/zh";
import { zhTW } from "./locales/zh-TW";
import { ja } from "./locales/ja";
import { ko } from "./locales/ko";
import { es } from "./locales/es";
import { fr } from "./locales/fr";
import { de } from "./locales/de";
import {
  type Query,
  type Selection,
  diffWorkspace,
  fileNavigation,
  presentDiff,
  useDiffRead,
} from "./diffRead";
import { type PendingFocus, useDiffFocusRestoration } from "./diffFocus";
import {
  DiffDetails,
  DiffFileReader,
  DiffOverview,
  DiffManifestNotices,
  DiffReadStatus,
  DiffReaderNavigation,
  DiffReaderToolbar,
  DiffScopeToolbar,
  fileSummary,
} from "./diffViews";

const CATALOGS = { en, zh, "zh-TW": zhTW, ja, ko, es, fr, de };

export function WorkspaceDiff({ context }: { context: ComponentContext }) {
  const t = useTranslator(CATALOGS, context.locale);
  const {
    runtime,
    workspace: capturedWorkspace,
    signal: componentSignal,
    track,
  } = context;
  const workspace = useMemo(
    () => diffWorkspace(capturedWorkspace),
    [capturedWorkspace],
  );
  const [query, setQuery] = useState<Query>(() => ({
    mode: "worktree",
    ...(workspace.focusPath ? { path: workspace.focusPath } : {}),
  }));
  const timeFormat = useMemo(
    () => new Intl.DateTimeFormat(context.locale, { timeStyle: "medium" }),
    [context.locale],
  );
  const [filter, setFilter] = useState("");
  const [layout, setLayout] = useState<"unified" | "split">("unified");
  const [selection, setSelection] = useState<Selection>(() =>
    workspace.focusPath
      ? { view: "file", path: workspace.focusPath, returnTo: "source" }
      : { view: "files", path: null },
  );

  const bodyViewport = useRef<HTMLDivElement>(null);
  const list = useRef<HTMLElement>(null);
  const search = useRef<HTMLInputElement>(null);
  const heading = useRef<HTMLHeadingElement>(null);
  const refreshButton = useRef<HTMLButtonElement>(null);
  const overviewScroll = useRef(0);
  const lastFocus = useRef<HTMLElement>(null);
  const pendingFocus = useRef<PendingFocus | null>(null);

  const { read, refresh } = useDiffRead({
    runtime,
    signal: componentSignal,
    track,
    workspace,
    query,
    onAccepted: (manifest) =>
      setSelection((current) =>
        current.view === "file" &&
        !manifest.some((file) => file.path === current.path)
          ? { path: current.path, view: "files" }
          : current,
      ),
    onFailed: () => {
      pendingFocus.current = null;
    },
  });

  const {
    accepted,
    files,
    hasFiles,
    truncated,
    selected,
    file,
    readableRendering,
    returnsToFiles,
    readingOverview,
    scoped,
    refreshFailed,
  } = presentDiff(read, selection, query);
  const filePath = file?.path;
  const fileChanges = file ? t("file.changes", { path: file.path }) : undefined;
  const patchActions = {
    wrap: t("patch.wrap"),
    unwrap: t("patch.unwrap"),
  };
  const copyPatch = useCopy(accepted?.diff.visual.patch ?? "", t("patch.copy"));
  const showingReader = file !== undefined;
  const { matching, navigationFiles, index } = fileNavigation(
    files,
    filter,
    file?.path,
  );

  function filterFiles(next: string) {
    if (next === filter) return;
    setFilter(next);
    if (bodyViewport.current) bodyViewport.current.scrollTop = 0;
  }
  function openFile(path: string, navigation?: HTMLButtonElement) {
    if (!file) overviewScroll.current = bodyViewport.current?.scrollTop ?? 0;
    const active = document.activeElement;
    pendingFocus.current =
      navigation && navigation === active && !navigation.disabled
        ? { target: "navigation", active: navigation }
        : { target: "file", active };
    setSelection({
      path,
      view: "file",
      returnTo: selection.view === "file" ? selection.returnTo : "files",
    });
  }
  function showFiles() {
    const workspaceQuery =
      !returnsToFiles &&
      (query.path !== undefined || accepted?.query.path !== undefined)
        ? { mode: query.mode }
        : undefined;
    pendingFocus.current = {
      target: "list",
      active: document.activeElement,
      query: workspaceQuery,
    };
    setSelection((current) => ({ path: current.path, view: "files" }));
    if (workspaceQuery) setQuery(workspaceQuery);
  }
  useLayoutEffect(() => {
    if (filePath !== undefined && bodyViewport.current) {
      bodyViewport.current.scrollTop = 0;
    }
  }, [filePath]);
  useDiffFocusRestoration(
    {
      pending: pendingFocus,
      last: lastFocus,
      refreshButton,
      search,
      heading,
      list,
      bodyViewport,
      overviewScroll,
    },
    { file, showingReader, accepted, hasFiles, selection, error: read.error },
  );

  return (
    <Panel
      bodyViewportRef={bodyViewport}
      label={t("panel.title")}
      onFocusCapture={(event) => {
        lastFocus.current = event.target;
      }}
      onKeyDown={(event) => {
        if (
          event.key !== "Escape" ||
          event.defaultPrevented ||
          isImeKey(event.nativeEvent) ||
          !showingReader ||
          !returnsToFiles
        )
          return;
        event.preventDefault();
        event.stopPropagation();
        showFiles();
      }}
      header={<PanelHeader size="md" title={t("panel.title")} />}
      controls={
        <>
          {!showingReader && (
            <DiffScopeToolbar
              t={t}
              scoped={scoped}
              mode={query.mode}
              readingOverview={readingOverview}
              loading={read.loading}
              failed={refreshFailed}
              refreshRef={refreshButton}
              onShowFiles={showFiles}
              onMode={(mode) =>
                setQuery((current) =>
                  current.mode === mode ? current : { ...current, mode },
                )
              }
              onRefresh={() => void refresh()}
            />
          )}
          {showingReader ? (
            <>
              <DiffReaderToolbar
                t={t}
                navigation={
                  file && (
                    <DiffReaderNavigation
                      t={t}
                      files={navigationFiles}
                      index={index}
                      filtered={Boolean(filter) && navigationFiles === matching}
                      changes={fileChanges}
                      onOpen={openFile}
                    />
                  )
                }
                failed={Boolean(read.error)}
                loading={read.loading}
                returnsToFiles={returnsToFiles}
                onShowFiles={showFiles}
                onRefresh={() => void refresh()}
              />
              {file && (
                <DiffFileHeader ref={heading} file={fileSummary(t, file)}>
                  {!file.binary && readableRendering && (
                    <>
                      <Select
                        size="sm"
                        label={t("layout.label")}
                        value={layout}
                        onValueChange={(value) =>
                          setLayout(value as "unified" | "split")
                        }
                        options={[
                          { value: "unified", label: t("layout.unified") },
                          { value: "split", label: t("layout.split") },
                        ]}
                      />
                      <CodeWrapButton labels={patchActions} size="sm" />
                      <IconButton
                        icon={Copy}
                        size="sm"
                        aria-label={t("patch.copy")}
                        onClick={() => void copyPatch()}
                      />
                    </>
                  )}
                </DiffFileHeader>
              )}
            </>
          ) : hasFiles ? (
            <FilterBar
              label={t("controls.label")}
              search={
                <SearchField
                  ref={search}
                  aria-label={t("filter.label")}
                  placeholder={t("filter.placeholder")}
                  value={filter}
                  onChange={(event) => filterFiles(event.target.value)}
                  onClear={() => filterFiles("")}
                  clearLabel={t("filter.clear")}
                />
              }
            />
          ) : null}
        </>
      }
    >
      <DiffReadStatus t={t} read={read} query={query} />
      {accepted && (
        <>
          <DiffManifestNotices
            t={t}
            truncated={truncated}
            hasFiles={hasFiles}
            missingPath={selected ? undefined : (selection.path ?? undefined)}
          />
          {hasFiles && (
            <div hidden={showingReader}>
              <DiffOverview
                t={t}
                files={files}
                matching={matching}
                filtered={Boolean(filter)}
                truncated={truncated}
                currentPath={selection.path}
                listRef={list}
                onSelect={(path) => openFile(path)}
              />
            </div>
          )}
          {file && (
            <DiffFileReader
              t={t}
              file={file}
              rendered={readableRendering}
              layout={layout}
              label={fileChanges}
            />
          )}
          <DiffDetails
            t={t}
            accepted={accepted}
            root={workspace.ref.path}
            timeFormat={timeFormat}
          />
        </>
      )}
    </Panel>
  );
}

export function WorktreeChanges({
  context,
  input,
}: {
  context: ComponentContext;
  input?: WorkspaceChangesActionInput;
}) {
  const labelId = useId();
  const t = useTranslator(CATALOGS, context.locale);
  const summary = summarizeWorkingTreeChanges(input?.changes);
  if (!input || !summary) return null;
  const counted =
    (summary.added !== undefined && summary.added !== 0) ||
    (summary.removed !== undefined && summary.removed !== 0);
  return (
    <Button
      size="sm"
      chip
      face="mono"
      title={t("worktree.open", { count: summary.files })}
      aria-labelledby={`${labelId} ${labelId}-value`}
      onClick={(event) => {
        context.signal.throwIfAborted();
        input.openView(event.currentTarget);
      }}
    >
      <span id={labelId} className="sr-only">
        {t("worktree.open", { count: summary.files })}
      </span>
      {counted ? (
        <DiffStat
          id={`${labelId}-value`}
          added={summary.added}
          removed={summary.removed}
        />
      ) : (
        <span id={`${labelId}-value`}>
          {t("worktree.files", { count: summary.files })}
        </span>
      )}
    </Button>
  );
}
