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
  FileDiff,
  List,
  Refresh,
} from "@flame/icons/react";
import type { Diff, DiffMode } from "@flame/runtime-client/wire";
import {
  Button,
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

type Read = {
  loading: boolean;
  accepted?: { diff: Diff; mode: DiffMode };
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
  const [mode, setMode] = useState<DiffMode>("worktree");
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
  } | null>(null);

  const refresh = useCallback(async () => {
    reading.current?.abort();
    const controller = new AbortController();
    reading.current = controller;
    const signal = AbortSignal.any([context.signal, controller.signal]);
    setRead((previous) => ({ ...previous, loading: true, error: undefined }));
    try {
      const diff = await context.track(
        context.runtime
          .workspace(workspace.ref)
          .diff.get({ mode, format: "rows" }, signal),
      );
      if (!signal.aborted) {
        setSelection((current) =>
          current.view === "file" &&
          !diff.files?.some((file) => file.path === current.path)
            ? { path: current.path, view: "files" }
            : current,
        );
        setRead({ loading: false, accepted: { diff, mode } });
      }
    } catch (error) {
      if (!signal.aborted)
        setRead((previous) => ({
          ...previous,
          loading: false,
          error: failureText(error, t("read.failed")),
        }));
    }
  }, [context, mode, workspace, t]);
  useEffect(() => {
    void refresh();
    return () => reading.current?.abort();
  }, [refresh]);

  const files = read.accepted?.diff.files ?? [];
  const selected = files.find((file) => file.path === selection.path);
  const file = selection.view === "file" ? selected : undefined;
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
    pendingFocus.current = { target: "list", active: document.activeElement };
    setSelection((current) => ({ path: current.path, view: "files" }));
  }
  useLayoutEffect(() => {
    const pending = pendingFocus.current;
    pendingFocus.current = null;
    const active = document.activeElement;
    if (!pending) {
      if (
        !file &&
        active === document.body &&
        lastFocus.current?.isConnected === false
      )
        search.current?.focus();
      return;
    }
    if (active !== pending.active && active !== document.body) return;
    if (pending.target === "file" && file) {
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
  }, [file, read.accepted, selection]);

  const accepted = read.accepted;
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
          !file ||
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
          {!file && (
            <Toolbar
              label={t("scope.label")}
              actions={
                <IconButton
                  icon={Refresh}
                  aria-label={t("refresh")}
                  size="md"
                  pending={read.loading}
                  onClick={() => void refresh()}
                />
              }
            >
              <Select
                label={t("comparison.label")}
                value={mode}
                options={COMPARISONS.map((value) => ({
                  value,
                  label: t(`comparison.${value}`),
                }))}
                onValueChange={(value) => setMode(value as DiffMode)}
              />
            </Toolbar>
          )}
          {read.loading && (
            <Metadata role="status">
              {t("status.reading", {
                name: comparisonName(t, mode).toLocaleLowerCase(),
              })}
            </Metadata>
          )}
          {read.error && (
            <Metadata role="alert">
              {t("status.error", {
                name: comparisonName(t, mode).toLocaleLowerCase(),
                error: read.error,
              })}
            </Metadata>
          )}
          {accepted &&
            (read.loading || read.error || accepted.mode !== mode) && (
              <Metadata role="status">
                {t("status.stale", {
                  name: comparisonName(t, accepted.mode).toLocaleLowerCase(),
                })}
              </Metadata>
            )}
          {file ? (
            <>
              <Toolbar
                label={t("navigation.label")}
                actions={
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
                      onClick={() => openFile(navigationFiles[index - 1]!.path)}
                    />
                    <IconButton
                      icon={ChevronRight}
                      aria-label={t("nav.next")}
                      size="sm"
                      disabled={
                        index < 0 || index >= navigationFiles.length - 1
                      }
                      onClick={() => openFile(navigationFiles[index + 1]!.path)}
                    />
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
              <DiffFileHeader ref={heading} file={file}>
                {!file.binary && file.rows.length > 0 && (
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
                  onChange={(event) => setFilter(event.target.value)}
                  onClear={() => setFilter("")}
                  clearLabel={t("filter.clear")}
                  disabled={!accepted}
                />
              }
            />
          )}
        </>
      }
    >
      {accepted && (
        <>
          {!read.loading && !read.error && accepted.mode === mode && (
            <Metadata role="status" className="sr-only">
              {t("status.loaded", {
                name: comparisonName(t, accepted.mode).toLocaleLowerCase(),
              })}
            </Metadata>
          )}
          {accepted.diff.truncated && (
            <Metadata role="note">{t("partial")}</Metadata>
          )}
          {selection.path && !selected && (
            <Metadata>
              {t("missing")} <DiffFilePath path={selection.path} />
            </Metadata>
          )}
          <div hidden={Boolean(file)}>
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
                  icon={FileDiff}
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
                files={matching}
                label={t("files.region")}
                currentPath={selection.path}
                onSelect={openFile}
              />
            </PanelSection>
          </div>
          {file && (
            <section aria-label={t("file.changes", { path: file.path })}>
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
          <Disclosure title={t("details.title")}>
            <Metadata>
              {accepted.diff.baseline.type === "emptyTree"
                ? t("details.emptyTree")
                : accepted.mode === "worktree"
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
