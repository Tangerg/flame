import {
  Fragment,
  useCallback,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
} from "react";
import type { ComponentContext } from "@flame/plugin-sdk";
import { FileDiff, Fold, Refresh, UnfoldVertical } from "@flame/icons/react";
import type { Diff, DiffMode } from "@flame/runtime-contract/wire";
import {
  Badge,
  DiffTable,
  Disclosure,
  EmptyState,
  Metadata,
  Panel,
  PanelHeader,
  PanelSection,
  IconButton,
  RecordSummary,
  RecordGroup,
  SearchField,
  FilterBar,
  Select,
  Toolbar,
} from "@flame/ui";

export type DiffSnapshot = { diff: Diff; focusPath?: string };

function FilePath({ path }: { path: string }) {
  const cut = path.lastIndexOf("/") + 1;
  return (
    <span dir="ltr">
      {cut > 0 && (
        <span style={{ color: "var(--color-text-muted)" }}>
          {path
            .slice(0, cut)
            .split("/")
            .slice(0, -1)
            .map((segment, index) => (
              <Fragment key={index}>
                {segment}/<wbr />
              </Fragment>
            ))}
        </span>
      )}
      {path.slice(cut)}
    </span>
  );
}
function DiffReport({
  snapshot,
  filter,
  layout,
  collapsed,
  onOpenChange,
}: {
  snapshot: DiffSnapshot;
  filter: string;
  layout: "unified" | "split";
  collapsed: ReadonlySet<string>;
  onOpenChange: (path: string, open: boolean) => void;
}) {
  const focused = useRef<HTMLElement>(null);
  const focusId = useId();
  useEffect(() => {
    focused.current?.scrollIntoView({ block: "nearest" });
  }, [snapshot]);
  const files = snapshot.diff.files ?? [];
  const matching = files.filter((file) =>
    `${file.path} ${file.previousPath ?? ""}`
      .toLocaleLowerCase()
      .includes(filter.toLocaleLowerCase()),
  );
  const source = snapshot.diff.baseline;
  return (
    <>
      {snapshot.diff.truncated && (
        <Metadata>
          <span id="truncated">
            This is a partial diff. Some changes were not included in this read.
          </span>
        </Metadata>
      )}
      <PanelSection
        title="Changed files"
        description={`${matching.length} of ${files.length} files in this read`}
      >
        {!files.length && !snapshot.diff.truncated && (
          <EmptyState
            icon={FileDiff}
            size="compact"
            title="No changes in this comparison."
            sub="Choose another baseline or refresh after editing a file."
          />
        )}
        {files.length > 0 && !matching.length && (
          <EmptyState
            size="compact"
            title="No matching files."
            sub="Clear the filter to show changed files."
          />
        )}
        {snapshot.focusPath &&
          !files.some((file) => file.path === snapshot.focusPath) && (
            <Metadata>The requested file is absent from this diff.</Metadata>
          )}
        <RecordGroup variant="list" id="files" aria-label="Changed files">
          {matching.map((file) => (
            <Disclosure
              variant="row"
              stickyHeader
              key={file.path}
              ref={file.path === snapshot.focusPath ? focused : undefined}
              id={file.path === snapshot.focusPath ? focusId : undefined}
              data-diff-file={file.path}
              data-focused={file.path === snapshot.focusPath}
              open={!collapsed.has(file.path)}
              onOpenChange={(open) => onOpenChange(file.path, open)}
              title={
                <RecordSummary
                  title={<FilePath path={file.path} />}
                  description={
                    <>
                      {file.status}{" "}
                      {!file.binary && (
                        <Badge
                          tone="neutral"
                          face="mono"
                        >{`+${file.added ?? "?"} −${file.removed ?? "?"}`}</Badge>
                      )}
                    </>
                  }
                />
              }
            >
              {file.previousPath && (
                <Metadata>
                  From <code>{file.previousPath}</code>
                </Metadata>
              )}
              {file.binary ? (
                <Metadata>Binary content is not displayed.</Metadata>
              ) : (
                <>
                  <DiffTable
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
                  <Metadata>Scroll horizontally to read long lines.</Metadata>
                </>
              )}
            </Disclosure>
          ))}
        </RecordGroup>
        <Disclosure title="Comparison details">
          <Metadata>
            <span id="baseline">
              {source.type === "emptyTree"
                ? "Baseline: empty tree"
                : `Baseline: ${source.type} · ${source.commit}`}
            </span>
          </Metadata>
        </Disclosure>
      </PanelSection>
      <Disclosure title="About this diff">
        <Metadata>
          These are current workspace changes, not a stored Tool result. Refresh
          to read again. This page cannot edit files, stage changes or restore a
          checkpoint.
        </Metadata>
      </Disclosure>
    </>
  );
}

type Read = {
  loading: boolean;
  accepted?: { snapshot: DiffSnapshot; mode: DiffMode };
  error?: string;
};

export function WorkspaceDiff({ context }: { context: ComponentContext }) {
  const workspace = useMemo(() => {
    if (!context.workspace || !context.workspace.focus)
      throw new Error("Workspace diff requires a workspace context");
    return { ...context.workspace, focus: context.workspace.focus };
  }, [context]);
  const [mode, setMode] = useState<DiffMode>("worktree");
  const [filter, setFilter] = useState("");
  const [layout, setLayout] = useState<"unified" | "split">("unified");
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(
    () => new Set(),
  );
  const [read, setRead] = useState<Read>({ loading: true });
  const reading = useRef<AbortController>(undefined);
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
      const prefix = workspace.ref.path.endsWith("/")
        ? workspace.ref.path
        : `${workspace.ref.path}/`;
      const focusPath = workspace.focus.path.startsWith(prefix)
        ? workspace.focus.path.slice(prefix.length)
        : workspace.focus.path;
      if (!signal.aborted) {
        setCollapsed((current) => {
          if (!current.has(focusPath)) return current;
          const next = new Set(current);
          next.delete(focusPath);
          return next;
        });
        setRead({
          loading: false,
          accepted: { snapshot: { diff, focusPath }, mode },
        });
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
  const files = read.accepted?.snapshot.diff.files ?? [];
  return (
    <Panel
      label="Workspace diff"
      header={
        <PanelHeader
          size="md"
          title="Workspace diff"
          context={
            <Metadata>
              <code>{workspace.ref.path}</code>
            </Metadata>
          }
        />
      }
      controls={
        <FilterBar
          label="Diff controls"
          search={
            <SearchField
              id="filter"
              aria-label="Filter files"
              placeholder="File path"
              value={filter}
              onChange={(event) => setFilter(event.target.value)}
            />
          }
        >
          <Select
            label="Comparison"
            value={mode}
            options={[
              { value: "worktree", label: "Working tree" },
              { value: "base", label: "Default branch" },
            ]}
            onValueChange={(value) => setMode(value as DiffMode)}
          />
          <Select
            id="layout"
            label="Layout"
            value={layout}
            onValueChange={(value) => setLayout(value as "unified" | "split")}
            options={[
              { value: "unified", label: "Unified" },
              { value: "split", label: "Side by side" },
            ]}
          />
          <Toolbar label="Diff actions">
            <IconButton
              icon={Refresh}
              aria-label="Refresh"
              size="md"
              disabled={read.loading}
              onClick={() => void refresh()}
            />
            <IconButton
              id="expand"
              icon={UnfoldVertical}
              aria-label="Expand all"
              size="md"
              disabled={!files.length}
              onClick={() => setCollapsed(new Set())}
            />
            <IconButton
              id="collapse"
              icon={Fold}
              aria-label="Collapse all"
              size="md"
              disabled={!files.length}
              onClick={() =>
                setCollapsed(new Set(files.map((file) => file.path)))
              }
            />
          </Toolbar>
        </FilterBar>
      }
    >
      {read.loading && <p role="status">Reading workspace diff…</p>}
      {read.error && (
        <p role="alert">The current diff could not be read: {read.error}</p>
      )}
      {read.accepted && (
        <>
          <Metadata
            role="status"
            className={read.accepted.mode === mode ? "sr-only" : undefined}
          >
            Workspace diff loaded.{" "}
            {read.accepted.mode === "worktree"
              ? "Working tree"
              : "Default branch"}{" "}
            comparison.
            {read.accepted.mode !== mode &&
              " Showing the last accepted comparison while the selected comparison is unavailable."}
          </Metadata>
          <DiffReport
            snapshot={read.accepted.snapshot}
            filter={filter}
            layout={layout}
            collapsed={collapsed}
            onOpenChange={(path, open) =>
              setCollapsed((current) => {
                const next = new Set(current);
                if (open) next.delete(path);
                else next.add(path);
                return next;
              })
            }
          />
        </>
      )}
    </Panel>
  );
}
