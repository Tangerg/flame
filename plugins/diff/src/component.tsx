import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { ComponentContext } from "@flame/plugin-sdk";
import { FileDiff, Refresh } from "@flame/icons/react";
import type { Diff, DiffMode } from "@flame/runtime-contract/wire";
import {
  Badge,
  Button,
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
export function DiffReport({ snapshot }: { snapshot: DiffSnapshot }) {
  const [filter, setFilter] = useState("");
  const [layout, setLayout] = useState("unified");
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(
    () => new Set(),
  );
  const focused = useRef<HTMLElement>(null);
  useEffect(() => {
    if (snapshot.focusPath)
      setCollapsed((current) => {
        const next = new Set(current);
        next.delete(snapshot.focusPath!);
        return next;
      });
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
        actions={
          <Toolbar label="File disclosure actions">
            <Button
              id="expand"
              size="sm"
              variant="ghost"
              onClick={() => setCollapsed(new Set())}
            >
              Expand all
            </Button>
            <Button
              id="collapse"
              size="sm"
              variant="ghost"
              onClick={() =>
                setCollapsed(new Set(files.map((file) => file.path)))
              }
            >
              Collapse all
            </Button>
          </Toolbar>
        }
      >
        <FilterBar
          label="Diff filters"
          search={
            <SearchField
              size="lg"
              id="filter"
              aria-label="Filter files"
              placeholder="File path"
              value={filter}
              onChange={(event) => setFilter(event.target.value)}
            />
          }
        >
          <Select
            size="lg"
            id="layout"
            label="Layout"
            value={layout}
            onValueChange={setLayout}
            options={[
              { value: "unified", label: "Unified" },
              { value: "split", label: "Side by side" },
            ]}
          />
        </FilterBar>
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
              key={file.path}
              ref={file.path === snapshot.focusPath ? focused : undefined}
              data-diff-file={file.path}
              data-focused={file.path === snapshot.focusPath}
              open={!collapsed.has(file.path)}
              onOpenChange={(open) =>
                setCollapsed((current) => {
                  const next = new Set(current);
                  if (open) next.delete(file.path);
                  else next.add(file.path);
                  return next;
                })
              }
              title={
                <RecordSummary
                  icon={FileDiff}
                  title={file.path.split("/").at(-1)}
                  description={
                    <code>
                      {file.status} · {file.path}
                    </code>
                  }
                  status={
                    !file.binary && (
                      <Badge
                        tone="neutral"
                        face="mono"
                      >{`+${file.added ?? "?"} −${file.removed ?? "?"}`}</Badge>
                    )
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
                    layout={layout as "unified" | "split"}
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
      if (!signal.aborted)
        setRead({
          loading: false,
          accepted: { snapshot: { diff, focusPath }, mode },
        });
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
  return (
    <Panel label="Workspace diff">
      <PanelHeader
        title="Workspace diff"
        context={
          <>
            <Toolbar label="Diff comparison actions">
              <Select
                label="Comparison"
                value={mode}
                options={[
                  { value: "worktree", label: "Working tree" },
                  { value: "base", label: "Default branch" },
                ]}
                onValueChange={(value) => setMode(value as DiffMode)}
              />
            </Toolbar>
            <Metadata>
              <code>{workspace.ref.path}</code>
            </Metadata>{" "}
          </>
        }
        description="Review changes in your workspace."
        actions={
          <IconButton
            icon={Refresh}
            aria-label="Refresh"
            size="md"
            disabled={read.loading}
            onClick={() => void refresh()}
          />
        }
      />

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
          <DiffReport snapshot={read.accepted.snapshot} />
        </>
      )}
    </Panel>
  );
}
