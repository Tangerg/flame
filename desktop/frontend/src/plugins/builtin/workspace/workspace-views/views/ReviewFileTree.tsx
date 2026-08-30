import { useState } from "react";
import type { KeyboardEvent, ReactNode } from "react";
import { DiffStat, FilePath, Icon, IconButton, Pressable, ScrollArea, TextField } from "@/ui";
import { AgentViewNavigator } from "@/ui/agent";
import { cn } from "@/lib/classNames";
import { useT } from "@/lib/i18n";
import type { WorkspaceFileDiff } from "@/plugins/builtin/workspace/application/workspaceQueries";
import {
  type ReviewTreeNode,
  buildReviewFileTree,
  filterReviewFiles,
} from "@/plugins/builtin/workspace/application/reviewFileTree";

function indentStyle(depth: number) {
  return { paddingLeft: `${0.5 + depth * 0.75}rem` };
}

const NOTHING_COLLAPSED: ReadonlySet<string> = new Set();

function TreeRows({
  nodes,
  depth,
  selectedPath,
  collapsedPaths,
  binaryLabel,
  onToggleDirectory,
  onSelectFile,
}: {
  binaryLabel: string;
  nodes: ReviewTreeNode[];
  depth: number;
  selectedPath: string;
  collapsedPaths: ReadonlySet<string>;
  onToggleDirectory: (path: string) => void;
  onSelectFile: (path: string) => void;
}) {
  return nodes.map((node) => {
    if (node.kind === "file") {
      return (
        <TreeRow
          key={`file:${node.path}`}
          depth={depth}
          selected={node.path === selectedPath}
          leading={<Icon name="file" size="sm" className="shrink-0 opacity-70" />}
          label={node.name}
          title={node.name}
          trailing={
            <DiffStat
              added={node.added}
              removed={node.removed}
              binary={node.binary ? binaryLabel : undefined}
            />
          }
          onClick={() => onSelectFile(node.path)}
        />
      );
    }
    const open = !collapsedPaths.has(node.path);
    return (
      <div key={`dir:${node.path}`}>
        <TreeRow
          depth={depth}
          selected={false}
          leading={
            <Icon
              name="chevron-down"
              size="xs"
              className={cn("shrink-0 transition-transform", !open && "-rotate-90")}
            />
          }
          label={<FilePath path={node.name} className="text-fg-muted" />}
          title={node.name}
          expanded={open}
          trailing={
            <DiffStat
              added={node.added}
              removed={node.removed}
              binary={node.binary ? binaryLabel : undefined}
            />
          }
          onClick={() => onToggleDirectory(node.path)}
        />
        {open && (
          <TreeRows
            nodes={node.children}
            depth={depth + 1}
            selectedPath={selectedPath}
            collapsedPaths={collapsedPaths}
            binaryLabel={binaryLabel}
            onToggleDirectory={onToggleDirectory}
            onSelectFile={onSelectFile}
          />
        )}
      </div>
    );
  });
}

function TreeRow({
  depth,
  selected,
  leading,
  label,
  title,
  expanded,
  trailing,
  onClick,
}: {
  depth: number;
  selected: boolean;
  leading: ReactNode;
  label: ReactNode;
  title: string;
  expanded?: boolean;
  trailing?: ReactNode;
  onClick: () => void;
}) {
  return (
    <Pressable
      type="button"
      data-chrome-focus=""
      aria-expanded={expanded}
      aria-current={selected ? "true" : undefined}
      onClick={onClick}
      style={indentStyle(depth)}
      className={cn(
        "flex h-7 w-full min-w-0 items-center gap-1.5 rounded-md border-0 bg-transparent pr-2",
        "text-left text-ui-xs text-fg transition-colors hover:bg-hover focus-visible:bg-hover",
        selected && "bg-selected",
      )}
      title={title}
    >
      {leading}
      <span className="min-w-0 flex-1 truncate">{label}</span>
      {trailing}
    </Pressable>
  );
}

export function ReviewFileTree({
  files,
  selectedPath,
  onSelectFile,
  onClose,
}: {
  files: WorkspaceFileDiff[];
  selectedPath: string;
  onSelectFile: (path: string) => void;
  onClose: () => void;
}) {
  const t = useT();
  const [query, setQuery] = useState("");
  const [collapsedPaths, setCollapsedPaths] = useState<ReadonlySet<string>>(() => new Set());

  const filtered = filterReviewFiles(files, query);
  const nodes = buildReviewFileTree(filtered);
  const filtering = query.trim().length > 0;

  const toggleDirectory = (path: string) => {
    setCollapsedPaths((previous) => {
      const next = new Set(previous);
      if (!next.delete(path)) next.add(path);
      return next;
    });
  };
  const onFilterKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === "Escape" && query.length > 0) {
      event.stopPropagation();
      setQuery("");
    }
  };

  return (
    <AgentViewNavigator
      label={t("diff.files.aria")}
      header={
        <>
          <TextField
            size="sm"
            font="sans"
            value={query}
            spellCheck={false}
            autoComplete="off"
            placeholder={t("diff.files.filter")}
            aria-label={t("diff.files.filter")}
            onChange={(event) => setQuery(event.target.value)}
            onKeyDown={onFilterKeyDown}
          />
          <IconButton icon="x" size="sm" aria-label={t("diff.files.hide")} onClick={onClose} />
        </>
      }
    >
      <ScrollArea className="px-1 py-1">
        {nodes.length === 0 ? (
          <p className="m-0 px-2 py-2 text-ui-xs text-fg-faint">
            {filtering ? t("diff.files.noMatch") : t("diff.files.none")}
          </p>
        ) : (
          <TreeRows
            nodes={nodes}
            depth={0}
            selectedPath={selectedPath}
            binaryLabel={t("files.binary")}
            collapsedPaths={filtering ? NOTHING_COLLAPSED : collapsedPaths}
            onToggleDirectory={toggleDirectory}
            onSelectFile={onSelectFile}
          />
        )}
      </ScrollArea>
    </AgentViewNavigator>
  );
}
