import { summarizeWorkingTreeChanges } from "./workingTreeChanges";
import { parseVisualDiff } from "./visualDiff";
import {
  useCallback,
  useEffect,
  useId,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type RefObject,
} from "react";
import {
  failureText,
  type ComponentContext,
  type WorkspaceChangesActionInput,
  type Translate,
  useTranslator,
} from "@flame/component-sdk";
import {
  ArrowLeft,
  ChevronLeft,
  ChevronRight,
  Copy,
  FileDiff as FileDiffIcon,
  List,
  Refresh,
} from "@flame/icons/react";
import type { DiffMode, WorkspaceFileChange } from "@flame/runtime-client/wire";
import {
  Button,
  CodeWrapButton,
  DiffBody,
  DiffFileHeader,
  DiffFileList,
  DiffFilePath,
  DiffStat,
  Disclosure,
  EmptyState,
  FilterBar,
  Icon,
  IconButton,
  isImeKey,
  Metadata,
  Panel,
  PanelHeader,
  PanelSection,
  SearchField,
  Hang,
  Select,
  Toolbar,
  useCopy,
  type DiffFileSummary,
} from "@flame/ui";
import { en } from "./locales/en";
import { zh } from "./locales/zh";
import { zhTW } from "./locales/zh-TW";
import { ja } from "./locales/ja";
import { ko } from "./locales/ko";
import { es } from "./locales/es";
import { fr } from "./locales/fr";
import { de } from "./locales/de";

const CATALOGS = { en, zh, "zh-TW": zhTW, ja, ko, es, fr, de };

const COMPARISONS: DiffMode[] = ["worktree", "base"];
const comparisonName = (t: Translate, mode: DiffMode) =>
  t(`comparison.${mode}`);

const fileSummary = (
  t: Translate,
  file: WorkspaceFileChange,
): DiffFileSummary => ({
  path: file.path,
  status:
    file.status === "modified" &&
    !file.binary &&
    file.added !== undefined &&
    file.removed !== undefined
      ? undefined
      : t(`file.status.${file.status}`),
  added: file.added,
  removed: file.removed,
  binary: file.binary,
  previous: file.previousPath
    ? { path: file.previousPath, label: t("file.renamed") }
    : undefined,
});

type Query = { mode: DiffMode; path?: string };
type Read = {
  loading: boolean;
  accepted?: ReturnType<typeof parseVisualDiff> & { query: Query; at: string };
  error?: { cause: unknown };
};
type Selection =
  | { view: "files"; path: string | null }
  | { view: "file"; path: string; returnTo: "files" | "source" };
type PendingFocus =
  | { target: "list"; active: Element | null; query?: Query }
  | { target: "file"; active: Element | null }
  | { target: "navigation"; active: HTMLButtonElement };

function restoreFileOverview(
  list: HTMLElement | null,
  fallback: HTMLElement | null,
  selectedPath: string | null,
) {
  const row = Array.from(
    list?.querySelectorAll<HTMLButtonElement>("[data-diff-file]") ?? [],
  ).find((item) => item.dataset.diffFile === selectedPath);
  (row ?? fallback)?.focus();
}

// A connected origin distinguishes deliberate blur from its removal.
function focusMovedFromOrigin(
  active: Element | null,
  origin: Element | null,
  body: HTMLElement,
) {
  return active !== origin && (active !== body || origin?.isConnected === true);
}

interface DiffFocusTargets {
  pending: RefObject<PendingFocus | null>;
  last: RefObject<HTMLElement | null>;
  refreshButton: RefObject<HTMLButtonElement | null>;
  search: RefObject<HTMLInputElement | null>;
  heading: RefObject<HTMLHeadingElement | null>;
  list: RefObject<HTMLElement | null>;
  bodyViewport: RefObject<HTMLDivElement | null>;
  overviewScroll: RefObject<number>;
}

// Switching between the overview and the reader replaces the element that held
// focus; this returns focus to the control that matches the requested move.
function useDiffFocusRestoration(
  targets: DiffFocusTargets,
  state: {
    file: unknown;
    showingReader: boolean;
    accepted: Read["accepted"];
    hasFiles: boolean;
    selection: Selection;
    error: unknown;
  },
) {
  useLayoutEffect(() => {
    const pending = targets.pending.current;
    const active = document.activeElement;
    const fallback =
      state.accepted && !state.hasFiles
        ? targets.refreshButton.current
        : targets.search.current;
    const lostFocus = targets.last.current?.isConnected === false;
    if (lostFocus) targets.last.current = null;
    if (!pending) {
      if (!state.showingReader && active === document.body && lostFocus)
        fallback?.focus();
      return;
    }
    const navigation = pending.target === "navigation" ? pending.active : null;
    // Disabling the activated navigation button can blur it to the body.
    const lostNavigationFocus =
      state.showingReader && navigation?.disabled && active === document.body;
    if (
      !lostNavigationFocus &&
      focusMovedFromOrigin(active, pending.active, document.body)
    ) {
      targets.pending.current = null;
      return;
    }
    if (
      pending.target === "list" &&
      pending.query &&
      state.accepted?.query !== pending.query
    ) {
      // Refresh keeps focus while the requested overview is pending, even
      // when an earlier scoped result still provides visible rows.
      if (
        active === document.body &&
        pending.active?.isConnected === false &&
        targets.refreshButton.current
      ) {
        targets.refreshButton.current.focus();
        targets.pending.current = {
          ...pending,
          active: targets.refreshButton.current,
        };
      }
      return;
    }
    targets.pending.current = null;
    if (pending.target !== "list" && state.showingReader) {
      if (!navigation?.isConnected || navigation.disabled) {
        targets.heading.current?.focus();
      }
    } else {
      if (targets.bodyViewport.current)
        targets.bodyViewport.current.scrollTop = targets.overviewScroll.current;
      restoreFileOverview(targets.list.current, fallback, state.selection.path);
    }
    // The targets are stable refs; only the presented state reruns the restoration.
  }, [
    state.file,
    state.showingReader,
    state.accepted,
    state.hasFiles,
    state.selection,
    state.error,
  ]);
}

function DiffReadStatus({
  t,
  read,
  query,
}: {
  t: Translate;
  read: Read;
  query: Query;
}) {
  const accepted = read.accepted;
  const settled = !read.loading && !read.error;
  return (
    <>
      {read.loading && (
        <Metadata role="status">
          {t("status.reading", {
            name: comparisonName(t, query.mode).toLocaleLowerCase(),
          })}
        </Metadata>
      )}
      {read.error && (
        <Metadata role="alert">
          {t("status.error", {
            name: comparisonName(t, query.mode).toLocaleLowerCase(),
            error: failureText(read.error.cause, t("read.failed")),
          })}
        </Metadata>
      )}
      {accepted && (!settled || accepted.query !== query) && (
        <Metadata
          role="status"
          className={accepted.query.mode !== query.mode ? undefined : "sr-only"}
        >
          {t("status.stale", {
            name: comparisonName(t, accepted.query.mode).toLocaleLowerCase(),
          })}
        </Metadata>
      )}
      {accepted && settled && accepted.query === query && (
        <Metadata role="status" className="sr-only">
          {t("status.loaded", {
            name: comparisonName(t, accepted.query.mode).toLocaleLowerCase(),
          })}
        </Metadata>
      )}
    </>
  );
}

export function WorkspaceDiff({ context }: { context: ComponentContext }) {
  const t = useTranslator(CATALOGS, context.locale);
  const {
    runtime,
    workspace: capturedWorkspace,
    signal: componentSignal,
    track,
  } = context;
  const workspace = useMemo(() => {
    if (!capturedWorkspace || !capturedWorkspace.focus)
      throw new Error("Workspace diff requires a workspace context");
    const prefix = capturedWorkspace.ref.path.endsWith("/")
      ? capturedWorkspace.ref.path
      : `${capturedWorkspace.ref.path}/`;
    const path = capturedWorkspace.focus.path;
    return {
      ref: capturedWorkspace.ref,
      focusPath: path.startsWith(prefix) ? path.slice(prefix.length) : path,
    };
  }, [capturedWorkspace]);
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
  const [read, setRead] = useState<Read>({ loading: true });
  const reading = useRef<AbortController>(undefined);
  const bodyViewport = useRef<HTMLDivElement>(null);
  const list = useRef<HTMLElement>(null);
  const search = useRef<HTMLInputElement>(null);
  const heading = useRef<HTMLHeadingElement>(null);
  const refreshButton = useRef<HTMLButtonElement>(null);
  const overviewScroll = useRef(0);
  const lastFocus = useRef<HTMLElement>(null);
  const pendingFocus = useRef<PendingFocus | null>(null);

  const refresh = useCallback(async () => {
    reading.current?.abort();
    const controller = new AbortController();
    reading.current = controller;
    const signal = AbortSignal.any([componentSignal, controller.signal]);
    setRead((previous) => ({ ...previous, loading: true, error: undefined }));
    try {
      const diff = await track(() =>
        runtime
          .workspace(workspace.ref)
          .diff.get({ ...query, format: "visual" }, signal),
      );
      if (!signal.aborted) {
        const projected = parseVisualDiff(diff);
        const at = new Date().toISOString();
        setSelection((current) =>
          current.view === "file" &&
          !projected.diff.visual.manifest.some(
            (file) => file.path === current.path,
          )
            ? { path: current.path, view: "files" }
            : current,
        );
        setRead({ loading: false, accepted: { ...projected, query, at } });
      }
    } catch (error) {
      if (!signal.aborted) {
        pendingFocus.current = null;
        setRead((previous) => ({
          ...previous,
          loading: false,
          error: { cause: error },
        }));
      }
    }
  }, [runtime, componentSignal, track, query, workspace]);
  useEffect(() => {
    void refresh();
    return () => reading.current?.abort();
  }, [refresh]);

  const accepted = read.accepted;
  const files = accepted?.diff.visual.manifest ?? [];
  const hasFiles = files.length > 0;
  const truncated = accepted?.diff.visual.truncated === true;
  const selected = files.find((file) => file.path === selection.path);
  const file = selection.view === "file" ? selected : undefined;
  const filePath = file?.path;
  const fileChanges = file ? t("file.changes", { path: file.path }) : undefined;
  const rendered = file ? accepted?.rendered.get(file.path) : undefined;
  const readable = rendered !== undefined && rendered.file.hunks.length > 0;
  const patchActions = {
    wrap: t("patch.wrap"),
    unwrap: t("patch.unwrap"),
  };
  const copyPatch = useCopy(accepted?.diff.visual.patch ?? "", t("patch.copy"));
  const showingReader = file !== undefined;
  const returnsToFiles =
    selection.view === "file" && selection.returnTo === "files";
  const readingOverview =
    read.loading &&
    query.path === undefined &&
    accepted?.query.path !== undefined;
  const matching = files.filter((item) =>
    `${item.path} ${item.previousPath ?? ""}`
      .toLocaleLowerCase()
      .includes(filter.toLocaleLowerCase()),
  );
  const navigationFiles = matching.some((item) => item.path === file?.path)
    ? matching
    : files;
  const index = navigationFiles.findIndex((item) => item.path === file?.path);
  const textFiles = files.filter((item) => !item.binary);
  const total = (key: "added" | "removed") =>
    textFiles.every((item) => item[key] !== undefined)
      ? textFiles.reduce((sum, item) => sum + item[key]!, 0)
      : undefined;

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
            <Toolbar
              label={t("scope.label")}
              actions={
                <>
                  {(query.path || accepted?.query.path) && (
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={showFiles}
                      aria-label={t("files.show")}
                      pending={readingOverview}
                    >
                      <Icon glyph={List} size="sm" />
                      {t("files.button")}
                    </Button>
                  )}
                  {read.error && <Metadata>{t("read.failedShort")}</Metadata>}
                  <IconButton
                    ref={refreshButton}
                    icon={Refresh}
                    aria-label={t("refresh")}
                    size="md"
                    pending={read.loading}
                    onClick={() => void refresh()}
                  />
                </>
              }
            >
              <Hang>
                <Select
                  label={t("comparison.label")}
                  value={query.mode}
                  options={COMPARISONS.map((value) => ({
                    value,
                    label: t(`comparison.${value}`),
                  }))}
                  onValueChange={(value) =>
                    setQuery((current) =>
                      current.mode === value
                        ? current
                        : { ...current, mode: value as DiffMode },
                    )
                  }
                />
              </Hang>
            </Toolbar>
          )}
          {showingReader ? (
            <>
              <Toolbar
                label={t("navigation.label")}
                actions={
                  <>
                    {file && (
                      <>
                        <Metadata role="status">
                          <span aria-hidden="true">
                            {index + 1} / {navigationFiles.length}
                          </span>
                          <span className="sr-only">
                            {fileChanges}
                            {" · "}
                            {t(
                              filter && navigationFiles === matching
                                ? "navigation.positionMatching"
                                : "navigation.position",
                              {
                                position: index + 1,
                                total: navigationFiles.length,
                              },
                            )}
                          </span>
                        </Metadata>
                        <IconButton
                          icon={ChevronLeft}
                          aria-label={t("nav.previous")}
                          size="sm"
                          disabled={index <= 0}
                          onClick={(event) =>
                            openFile(
                              navigationFiles[index - 1]!.path,
                              event.currentTarget,
                            )
                          }
                        />
                        <IconButton
                          icon={ChevronRight}
                          aria-label={t("nav.next")}
                          size="sm"
                          disabled={
                            index < 0 || index >= navigationFiles.length - 1
                          }
                          onClick={(event) =>
                            openFile(
                              navigationFiles[index + 1]!.path,
                              event.currentTarget,
                            )
                          }
                        />
                      </>
                    )}
                    {read.error && <Metadata>{t("read.failedShort")}</Metadata>}
                    <IconButton
                      icon={Refresh}
                      aria-label={t("refresh")}
                      size="sm"
                      pending={read.loading}
                      onClick={() => void refresh()}
                    />
                  </>
                }
              >
                <Button
                  variant="ghost"
                  size="sm"
                  aria-label={
                    returnsToFiles ? t("files.back") : t("files.show")
                  }
                  onClick={showFiles}
                >
                  <Icon glyph={returnsToFiles ? ArrowLeft : List} size="sm" />
                  {t("files.button")}
                </Button>
              </Toolbar>
              {file && (
                <DiffFileHeader ref={heading} file={fileSummary(t, file)}>
                  {!file.binary && readable && (
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
          {truncated && hasFiles && (
            <Metadata role="note">{t("partial")}</Metadata>
          )}
          {!hasFiles && (
            <EmptyState
              icon={FileDiffIcon}
              size="compact"
              title={t(truncated ? "budget.empty" : "empty.title")}
              sub={truncated ? undefined : t("empty.sub")}
            />
          )}
          {selection.path && !selected && (
            <Metadata>
              {t("missing")} <DiffFilePath path={selection.path} />
            </Metadata>
          )}
          {hasFiles && (
            <div hidden={showingReader}>
              <PanelSection
                title={t(truncated ? "files.included" : "files.changed", {
                  count: files.length,
                })}
                description={
                  filter
                    ? t("files.match", {
                        count: matching.length,
                        total: files.length,
                      })
                    : undefined
                }
                actions={
                  textFiles.length > 0 ? (
                    <DiffStat
                      title={t("stat.title")}
                      added={total("added")}
                      removed={total("removed")}
                    />
                  ) : undefined
                }
              >
                {!matching.length && (
                  <EmptyState
                    size="compact"
                    title={t("nomatch.title")}
                    sub={t("nomatch.sub")}
                  />
                )}
                <DiffFileList
                  ref={list}
                  files={matching.map((item) => fileSummary(t, item))}
                  label={t("files.region")}
                  currentPath={selection.path}
                  onSelect={(path) => openFile(path)}
                />
              </PanelSection>
            </div>
          )}
          {file && (
            <section aria-label={fileChanges}>
              {file.binary ? (
                <Metadata>{t("file.binary")}</Metadata>
              ) : readable ? (
                <DiffBody
                  key={file.path}
                  patch={rendered.source}
                  layout={layout}
                  label={t("reader.region", { path: file.path })}
                />
              ) : (
                <Metadata>{t("file.noLineChanges")}</Metadata>
              )}
            </section>
          )}
          <Disclosure title={t("details.title")}>
            <Metadata>
              {accepted.diff.baseline.type === "emptyTree"
                ? t("details.emptyTree")
                : accepted.query.mode === "worktree"
                  ? t("details.worktree")
                  : t("details.base")}
            </Metadata>
            <Metadata>
              {accepted.diff.baseline.type === "emptyTree"
                ? t("baseline.emptyTree")
                : t("baseline.label", {
                    ref:
                      accepted.diff.baseline.type === "head"
                        ? t("baseline.head")
                        : t("baseline.mergeBase"),
                    commit: accepted.diff.baseline.commit,
                  })}
            </Metadata>
            <Metadata>
              <DiffFilePath path={workspace.ref.path} />
            </Metadata>
            <Metadata>
              <time dateTime={accepted.at}>
                {t("read.at", {
                  time: timeFormat.format(new Date(accepted.at)),
                })}
              </time>
            </Metadata>
          </Disclosure>
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
