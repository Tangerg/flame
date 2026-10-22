import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import type { ComponentContext } from "@flame/component-sdk";
import { scrollPositions, type ScrollPosition } from "@flame/component-sdk";
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

const COMPARISONS = [
  { value: "worktree", label: "Uncommitted changes" },
  { value: "base", label: "Branch changes" },
];
const comparisonName = (mode: DiffMode) =>
  COMPARISONS.find((comparison) => comparison.value === mode)?.label ?? mode;

type Read = {
  loading: boolean;
  accepted?: { diff: Diff; mode: DiffMode };
  error?: string;
};
type Selection =
  | { view: "files"; path: string | null }
  | { view: "file"; path: string; returnTo: "files" | "source" };

export function WorkspaceDiff({ context }: { context: ComponentContext }) {
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
          error:
            error instanceof Error
              ? error.message
              : "The diff could not be read.",
        }));
    }
  }, [context, mode, workspace]);
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
      label="Workspace diff"
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
          title="Workspace diff"
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
              label="Comparison scope"
              actions={
                <IconButton
                  icon={Refresh}
                  aria-label="Refresh"
                  size="md"
                  pending={read.loading}
                  onClick={() => void refresh()}
                />
              }
            >
              <Select
                label="Comparison"
                value={mode}
                options={COMPARISONS}
                onValueChange={(value) => setMode(value as DiffMode)}
              />
            </Toolbar>
          )}
          {read.loading && (
            <Metadata role="status">
              Reading {comparisonName(mode).toLocaleLowerCase()}…
            </Metadata>
          )}
          {read.error && (
            <Metadata role="alert">
              Could not read {comparisonName(mode).toLocaleLowerCase()}:{" "}
              {read.error}
            </Metadata>
          )}
          {accepted &&
            (read.loading || read.error || accepted.mode !== mode) && (
              <Metadata role="status">
                Showing {comparisonName(accepted.mode).toLocaleLowerCase()} from
                the last successful read.
              </Metadata>
            )}
          {file ? (
            <>
              <Toolbar
                label="Changed file navigation"
                actions={
                  <>
                    <Metadata
                      aria-live="polite"
                      aria-label={`File ${index + 1} of ${navigationFiles.length}${filter && navigationFiles === matching ? " matching files" : ""}`}
                    >
                      {index + 1} / {navigationFiles.length}
                    </Metadata>
                    <IconButton
                      icon={ChevronLeft}
                      aria-label="Previous file"
                      size="sm"
                      disabled={index <= 0}
                      onClick={() => openFile(navigationFiles[index - 1]!.path)}
                    />
                    <IconButton
                      icon={ChevronRight}
                      aria-label="Next file"
                      size="sm"
                      disabled={
                        index < 0 || index >= navigationFiles.length - 1
                      }
                      onClick={() => openFile(navigationFiles[index + 1]!.path)}
                    />
                    <IconButton
                      icon={Refresh}
                      aria-label="Refresh"
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
                      ? "Back to changed files"
                      : "Show changed files"
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
                  Files
                </Button>
              </Toolbar>
              <DiffFileHeader ref={heading} file={file}>
                {!file.binary && file.rows.length > 0 && (
                  <Select
                    size="sm"
                    label="Diff layout"
                    value={layout}
                    onValueChange={(value) =>
                      setLayout(value as "unified" | "split")
                    }
                    options={[
                      { value: "unified", label: "Unified" },
                      { value: "split", label: "Side by side" },
                    ]}
                  />
                )}
              </DiffFileHeader>
            </>
          ) : (
            <FilterBar
              label="File controls"
              search={
                <SearchField
                  ref={search}
                  aria-label="Filter files"
                  placeholder="Find a changed file"
                  value={filter}
                  onChange={(event) => setFilter(event.target.value)}
                  onClear={() => setFilter("")}
                  clearLabel="Clear filter"
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
              Showing {comparisonName(accepted.mode).toLocaleLowerCase()}.
            </Metadata>
          )}
          {accepted.diff.truncated && (
            <Metadata role="note">
              Partial diff. Only the included files and line changes are shown.
            </Metadata>
          )}
          {selection.path && !selected && (
            <Metadata>
              The requested file is not included in this read:{" "}
              <DiffFilePath path={selection.path} />
            </Metadata>
          )}
          <div hidden={Boolean(file)}>
            <PanelSection
              title={`${files.length} ${accepted.diff.truncated ? "included" : "changed"} ${files.length === 1 ? "file" : "files"}`}
              description={
                filter
                  ? `${matching.length} of ${files.length} files match`
                  : undefined
              }
              actions={
                textFiles.length > 0 ? (
                  <DiffStat
                    title="Line changes in included text files"
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
                  title="No changes in this comparison."
                  sub="Choose another comparison or refresh after editing a file."
                />
              )}
              {files.length > 0 && !matching.length && (
                <EmptyState
                  size="compact"
                  title="No matching files."
                  sub="Clear the filter to show changed files."
                />
              )}
              <DiffFileList
                ref={list}
                files={matching}
                label="Changed files"
                currentPath={selection.path}
                onSelect={openFile}
              />
            </PanelSection>
          </div>
          {file && (
            <section aria-label={`Changes in ${file.path}`}>
              {file.previousPath && (
                <Metadata>
                  Renamed from <DiffFilePath path={file.previousPath} />
                </Metadata>
              )}
              {file.binary ? (
                <Metadata>
                  Binary content is not displayed. Line counts are unavailable.
                </Metadata>
              ) : file.rows.length ? (
                <DiffTable
                  key={file.path}
                  rows={file.rows}
                  layout={layout}
                  label={file.path}
                  labels={{
                    region: `Scrollable diff for ${file.path}`,
                    beforeLine: "Before line",
                    beforeCode: "Before code",
                    afterLine: "After line",
                    afterCode: "After code",
                    changedCode: "Changed code",
                  }}
                />
              ) : (
                <Metadata>
                  No line changes. Only file metadata changed.
                </Metadata>
              )}
            </section>
          )}
          <Disclosure title="Comparison details">
            <Metadata>
              {accepted.diff.baseline.type === "emptyTree"
                ? "No commits yet. Changes are compared with an empty tree."
                : accepted.mode === "worktree"
                  ? "Changes against HEAD, including staged, unstaged and untracked files."
                  : "Committed and uncommitted tracked changes since the common ancestor with the default branch. Untracked files are excluded."}
            </Metadata>
            <Metadata>
              {accepted.diff.baseline.type === "emptyTree"
                ? "Baseline: empty tree"
                : `Baseline: ${accepted.diff.baseline.type === "head" ? "HEAD" : "merge base"} · ${accepted.diff.baseline.commit}`}
            </Metadata>
          </Disclosure>
        </>
      )}
    </Panel>
  );
}
