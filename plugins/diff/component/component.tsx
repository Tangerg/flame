import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  failureText,
  createTranslator,
  scrollPositions,
  type ComponentContext,
  type ScrollPosition,
  type Translate,
} from "@flame/component-sdk";
import {
  ArrowLeft,
  ChevronLeft,
  ChevronRight,
  FileDiff as FileDiffIcon,
  List,
  Refresh,
} from "@flame/icons/react";
import type { Diff, DiffMode, FileDiff } from "@flame/runtime-client/wire";
import {
  Button,
  CodeBlock,
  DiffFileHeader,
  DiffFileList,
  DiffFilePath,
  DiffStat,
  DiffTable,
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
  Select,
  Toolbar,
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

const fileSummary = (t: Translate, file: FileDiff): DiffFileSummary => ({
  path: file.path,
  status: t(`file.status.${file.status}`),
  added: file.added,
  removed: file.removed,
  binary: file.binary,
});

type Query =
  | { mode: DiffMode; format: "rows"; path?: string }
  | { mode: DiffMode; format: "raw"; path: string };
type Read = {
  loading: boolean;
  accepted?: { diff: Diff; query: Query; at: string };
  error?: string;
};
type Selection =
  | { view: "files"; path: string | null }
  | { view: "file"; path: string; returnTo: "files" | "source" };

export function WorkspaceDiff({ context }: { context: ComponentContext }) {
  const t = useMemo(
    () => createTranslator(CATALOGS, context.locale),
    [context.locale],
  );
  const workspace = useMemo(() => {
    if (!context.workspace || !context.workspace.focus)
      throw new Error("Workspace diff requires a workspace context");
    const prefix = context.workspace.ref.path.endsWith("/")
      ? context.workspace.ref.path
      : `${context.workspace.ref.path}/`;
    const path = context.workspace.focus.path;
    return {
      ref: context.workspace.ref,
      focusPath: path.startsWith(prefix) ? path.slice(prefix.length) : path,
    };
  }, [context]);
  const [query, setQuery] = useState<Query>(() => ({
    mode: "worktree",
    format: "rows",
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
  const panel = useRef<HTMLElement>(null);
  const list = useRef<HTMLElement>(null);
  const search = useRef<HTMLInputElement>(null);
  const heading = useRef<HTMLHeadingElement>(null);
  const overviewScroll = useRef<ScrollPosition[]>([]);
  const lastFocus = useRef<HTMLElement>(null);
  const pendingFocus = useRef<{
    target: "file" | "list";
    active: Element | null;
    query?: Query;
  } | null>(null);

  const refresh = useCallback(async () => {
    reading.current?.abort();
    const controller = new AbortController();
    reading.current = controller;
    const signal = AbortSignal.any([context.signal, controller.signal]);
    setRead((previous) => ({ ...previous, loading: true, error: undefined }));
    try {
      const diff = await context.track(
        context.runtime.workspace(workspace.ref).diff.get(query, signal),
      );
      if (!signal.aborted) {
        setSelection((current) =>
          query.format === "rows" &&
          current.view === "file" &&
          !diff.files?.some((file) => file.path === current.path)
            ? { path: current.path, view: "files" }
            : current,
        );
        setRead({
          loading: false,
          accepted: { diff, query, at: new Date().toISOString() },
        });
      }
    } catch (error) {
      if (!signal.aborted) {
        pendingFocus.current = null;
        setRead((previous) => ({
          ...previous,
          loading: false,
          error: failureText(error, t("read.failed")),
        }));
      }
    }
  }, [context, query, workspace, t]);
  useEffect(() => {
    void refresh();
    return () => reading.current?.abort();
  }, [refresh]);

  const accepted = read.accepted;
  const rawPath =
    accepted?.query.format === "raw" ? accepted.query.path : undefined;
  const files = accepted?.diff.files ?? [];
  const selected = files.find((file) => file.path === selection.path);
  const file = selection.view === "file" ? selected : undefined;
  const showingReader = Boolean(file || rawPath);
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
    for (const position of scrollPositions(panel.current))
      position.element.scrollTop = 0;
  }
  function openFile(path: string) {
    if (!file) overviewScroll.current = scrollPositions(panel.current);
    pendingFocus.current = { target: "file", active: document.activeElement };
    setSelection({
      path,
      view: "file",
      returnTo: selection.view === "file" ? selection.returnTo : "files",
    });
  }
  function showFiles() {
    const next: Query =
      query.path !== undefined || accepted?.query.path !== undefined
        ? { mode: query.mode, format: "rows" }
        : query;
    pendingFocus.current = {
      target: "list",
      active: document.activeElement,
      query: next !== query ? next : undefined,
    };
    setSelection((current) => ({ path: current.path, view: "files" }));
    setQuery(next);
  }
  function readTarget(format: "raw" | "rows") {
    const path = accepted?.query.path;
    if (!path) return;
    const next: Query = {
      ...accepted.query,
      format,
      path,
    };
    pendingFocus.current = {
      target: "file",
      active: document.activeElement,
      query: next,
    };
    setSelection({ view: "file", path, returnTo: "source" });
    setQuery(next);
  }
  useLayoutEffect(() => {
    const pending = pendingFocus.current;
    const active = document.activeElement;
    if (!pending) {
      if (
        !showingReader &&
        active === document.body &&
        lastFocus.current?.isConnected === false
      )
        search.current?.focus();
      return;
    }
    if (active !== pending.active && active !== document.body) {
      pendingFocus.current = null;
      return;
    }
    if (pending.query && accepted?.query !== pending.query) return;
    pendingFocus.current = null;
    if (pending.target === "file" && showingReader) {
      for (const position of scrollPositions(panel.current))
        position.element.scrollTop = 0;
      heading.current?.focus();
    } else {
      for (const position of overviewScroll.current)
        position.element.scrollTop = position.top;
      const row = Array.from(
        list.current?.querySelectorAll<HTMLButtonElement>("[data-diff-file]") ??
          [],
      ).find((item) => item.dataset.diffFile === selection.path);
      (row ?? search.current)?.focus();
    }
  }, [file, showingReader, accepted, selection, read.error]);

  return (
    <Panel
      ref={panel}
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
          selection.view !== "file" ||
          selection.returnTo !== "files"
        )
          return;
        event.preventDefault();
        event.stopPropagation();
        showFiles();
      }}
      header={
        <PanelHeader
          size="md"
          title={t("panel.title")}
          context={
            <Metadata>
              <DiffFilePath path={workspace.ref.path} />
            </Metadata>
          }
        />
      }
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
                    icon={Refresh}
                    aria-label={t("refresh")}
                    size="md"
                    pending={read.loading}
                    onClick={() => void refresh()}
                  />
                </>
              }
            >
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
                        <Metadata
                          aria-live="polite"
                          aria-label={
                            filter && navigationFiles === matching
                              ? t("navigation.positionMatching", {
                                  position: index + 1,
                                  total: navigationFiles.length,
                                })
                              : t("navigation.position", {
                                  position: index + 1,
                                  total: navigationFiles.length,
                                })
                          }
                        >
                          {index + 1} / {navigationFiles.length}
                        </Metadata>
                        <IconButton
                          icon={ChevronLeft}
                          aria-label={t("nav.previous")}
                          size="sm"
                          disabled={index <= 0}
                          onClick={() =>
                            openFile(navigationFiles[index - 1]!.path)
                          }
                        />
                        <IconButton
                          icon={ChevronRight}
                          aria-label={t("nav.next")}
                          size="sm"
                          disabled={
                            index < 0 || index >= navigationFiles.length - 1
                          }
                          onClick={() =>
                            openFile(navigationFiles[index + 1]!.path)
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
                    selection.view === "file" && selection.returnTo === "files"
                      ? t("files.back")
                      : t("files.show")
                  }
                  onClick={showFiles}
                  pending={readingOverview}
                >
                  <Icon
                    glyph={
                      selection.view === "file" &&
                      selection.returnTo === "files"
                        ? ArrowLeft
                        : List
                    }
                    size="sm"
                  />
                  {t("files.button")}
                </Button>
              </Toolbar>
              <DiffFileHeader
                ref={heading}
                file={file ? fileSummary(t, file) : { path: rawPath! }}
              >
                {rawPath ? (
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => readTarget("rows")}
                    pending={read.loading}
                  >
                    {t("raw.structured")}
                  </Button>
                ) : (
                  file &&
                  !file.binary &&
                  file.rows.length > 0 && (
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
                  )
                )}
              </DiffFileHeader>
            </>
          ) : (
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
                  disabled={!accepted}
                />
              }
            />
          )}
          {accepted && (
            <Metadata>
              {comparisonName(t, accepted.query.mode)}
              {" · "}
              {t(accepted.query.path ? "scope.file" : "scope.workspace")}
              {" · "}
              <span
                title={
                  accepted.diff.baseline.type === "emptyTree"
                    ? undefined
                    : accepted.diff.baseline.commit
                }
              >
                {accepted.diff.baseline.type === "emptyTree"
                  ? t("baseline.emptyTree")
                  : t("baseline.label", {
                      ref: t(
                        accepted.diff.baseline.type === "head"
                          ? "baseline.head"
                          : "baseline.mergeBase",
                      ),
                      commit: accepted.diff.baseline.commit.slice(0, 8),
                    })}
              </span>
              {" · "}
              <time dateTime={accepted.at}>
                {t("read.at", {
                  time: timeFormat.format(new Date(accepted.at)),
                })}
              </time>
            </Metadata>
          )}
        </>
      }
    >
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
            error: read.error,
          })}
        </Metadata>
      )}
      {accepted && (read.loading || read.error || accepted.query !== query) && (
        <Metadata role="status" className="sr-only">
          {t("status.stale", {
            name: comparisonName(t, accepted.query.mode).toLocaleLowerCase(),
          })}
        </Metadata>
      )}
      {accepted && (
        <>
          {!read.loading && !read.error && accepted.query === query && (
            <Metadata role="status" className="sr-only">
              {t("status.loaded", {
                name: comparisonName(
                  t,
                  accepted.query.mode,
                ).toLocaleLowerCase(),
              })}
            </Metadata>
          )}
          {accepted.diff.truncated && (
            <>
              <Metadata role="note">{t("partial")}</Metadata>
              {accepted.query.path && accepted.query.format === "rows" && (
                <Button
                  variant="soft"
                  size="sm"
                  onClick={() => readTarget("raw")}
                  pending={read.loading}
                >
                  {t("raw.read")}
                </Button>
              )}
            </>
          )}
          {selection.path && !selected && !rawPath && (
            <Metadata>
              {t("missing")} <DiffFilePath path={selection.path} />
            </Metadata>
          )}
          <div hidden={showingReader}>
            <PanelSection
              title={t(
                accepted.diff.truncated ? "files.included" : "files.changed",
                { count: files.length },
              )}
              description={
                filter
                  ? t("files.match", {
                      matching: matching.length,
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
              {!files.length && !accepted.diff.truncated && (
                <EmptyState
                  icon={FileDiffIcon}
                  size="compact"
                  title={t("empty.title")}
                  sub={t("empty.sub")}
                />
              )}
              {files.length > 0 && !matching.length && (
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
                onSelect={openFile}
              />
            </PanelSection>
          </div>
          {file && (
            <section aria-label={t("file.changes", { path: file.path })}>
              <Metadata>
                <DiffFilePath path={file.path} />
              </Metadata>
              {file.previousPath && (
                <Metadata>
                  {t("file.renamed")} <DiffFilePath path={file.previousPath} />
                </Metadata>
              )}
              {file.binary ? (
                <Metadata>{t("file.binary")}</Metadata>
              ) : file.rows.length ? (
                <DiffTable
                  key={file.path}
                  rows={file.rows}
                  layout={layout}
                  label={file.path}
                  labels={{
                    region: t("table.region", { path: file.path }),
                    beforeLine: t("table.beforeLine"),
                    beforeCode: t("table.beforeCode"),
                    afterLine: t("table.afterLine"),
                    afterCode: t("table.afterCode"),
                    changedCode: t("table.changedCode"),
                  }}
                />
              ) : (
                <Metadata>{t("file.metadataOnly")}</Metadata>
              )}
            </section>
          )}
          {rawPath && (
            <section aria-label={t("file.changes", { path: rawPath })}>
              <Metadata>
                <DiffFilePath path={rawPath} />
              </Metadata>
              {accepted.diff.patch ? (
                <CodeBlock
                  language="diff"
                  label={t("raw.region", { path: rawPath })}
                  labels={{
                    copy: t("raw.copy"),
                    copied: t("raw.copied"),
                    wrap: t("raw.wrap"),
                    unwrap: t("raw.unwrap"),
                  }}
                >
                  {accepted.diff.patch}
                </CodeBlock>
              ) : (
                <EmptyState
                  icon={FileDiffIcon}
                  size="compact"
                  title={t("empty.title")}
                  sub={t("empty.sub")}
                />
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
          </Disclosure>
        </>
      )}
    </Panel>
  );
}
