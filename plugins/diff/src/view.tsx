import { useEffect, useRef, useState } from "react";
import { FileDiff } from "@flame/icons/react";
import type { Diff } from "@flame/runtime-contract/wire";
import {
  Badge,
  Button,
  DiffTable,
  Disclosure,
  EmptyState,
  Metadata,
  RecordGroup,
  SearchField,
  Select,
  Toolbar,
} from "@flame/ui";
import {
  mountView,
  ReadStatus,
  useRead,
  ViewPanel,
  type ViewProps,
} from "../../src/view";

type DiffSnapshot = { diff: Diff; focusPath?: string };
function WorkspaceDiff({ view }: ViewProps<DiffSnapshot>) {
  const { data: snapshot } = useRead(view);
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
    focused.current?.scrollIntoView({ block: "start" });
  }, [snapshot]);
  const files = snapshot.diff.files ?? [];
  const matching = files.filter((file) =>
    `${file.path} ${file.previousPath ?? ""}`
      .toLocaleLowerCase()
      .includes(filter.toLocaleLowerCase()),
  );
  const source = snapshot.diff.baseline;
  return (
    <ViewPanel
      view={view}
      title="Workspace diff"
      description="Current workspace changes. Choose the comparison baseline with the controls above."
    >
      <Metadata>
        <span id="baseline">
          {source.type === "emptyTree"
            ? "Baseline: empty tree"
            : `Baseline: ${source.type} · ${source.commit}`}
        </span>
      </Metadata>
      <ReadStatus
        view={view}
        ready="Workspace diff loaded."
        loading="Reading workspace diff…"
      />
      {snapshot.diff.truncated && (
        <Metadata>
          <span id="truncated">
            This is a partial diff. Some changes were not included in this read.
          </span>
        </Metadata>
      )}
      <Toolbar label="Diff filters">
        <SearchField
          id="filter"
          aria-label="Filter files"
          placeholder="File path"
          value={filter}
          onChange={(event) => setFilter(event.target.value)}
        />
        <Select
          id="layout"
          label="Layout"
          value={layout}
          onValueChange={setLayout}
          options={[
            { value: "unified", label: "Unified" },
            { value: "split", label: "Side by side" },
          ]}
        />
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
          onClick={() => setCollapsed(new Set(files.map((file) => file.path)))}
        >
          Collapse all
        </Button>
      </Toolbar>
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
      <RecordGroup id="files" aria-label="Changed files">
        {matching.map((file) => (
          <Disclosure
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
              <>
                {file.previousPath
                  ? `${file.previousPath} → ${file.path}`
                  : file.path}{" "}
                · {file.status}{" "}
                {!file.binary && (
                  <Badge tone="neutral">{`+${file.added ?? "?"} −${file.removed ?? "?"}`}</Badge>
                )}
              </>
            }
          >
            {file.binary ? (
              <Metadata>Binary content is not displayed.</Metadata>
            ) : (
              <>
                <DiffTable
                  rows={file.rows}
                  layout={layout as "unified" | "split"}
                  label={file.path}
                />
                <Metadata>Scroll horizontally to read long lines.</Metadata>
              </>
            )}
          </Disclosure>
        ))}
      </RecordGroup>
      <Metadata>
        These are current workspace changes, not a stored Tool result. Refresh
        to read again. This page cannot edit files, stage changes or restore a
        checkpoint.
      </Metadata>
    </ViewPanel>
  );
}
mountView(WorkspaceDiff);
