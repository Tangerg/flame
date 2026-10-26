import { useWorkspace } from "@/features/workspace/public/workspace";
import * as stylex from "@stylexjs/stylex";
import { type Ref, useCallback, useId, useLayoutEffect, useRef, useState } from "react";
import { copyText } from "@flame/ui";
import { useT } from "@/lib/i18n/index";
import { ContextMenu } from "@/ui/index";
import { DataError, DataView, SkeletonList } from "@/ui/index";
import { Icon } from "@/ui/index";
import { Tree } from "@/ui/index";
import { chevron } from "@/ui/index";
import { fileIconName } from "@/ui/index";
import { vocab } from "@flame/ui";
import {
  rememberWorkspaceView,
  useWorkspaceViewMemory,
} from "@/features/workspace/application/navigation";
import {
  type WorkspaceFileChange,
  type WorkspaceFileEntry,
  useWorkspaceListFiles,
} from "@/features/workspace/application/workspaceQueries";
import { useWorkingTreeFiles } from "@/features/workspace/application/workingTreeChanges";

import { color, corner, space, type as typeStep } from "@flame/ui/tokens.stylex";
const CHANGE_MARK: Record<WorkspaceFileChange["change"], string> = {
  add: "A",
  mod: "M",
  del: "D",
  renamed: "R",
};

const ft = stylex.create({
  indent: { width: space.s3, flexShrink: 0 },
  pad: { paddingInline: space.s2, paddingBlock: space.s1_5 },
  name: { flex: 1 },
  mark: {
    flexShrink: 0,
    fontFamily: "var(--font-mono)",
    fontVariantNumeric: "tabular-nums",
  },
  add: { color: color.success },
  mod: { color: color.warning },
  del: { color: color.negative },
  renamed: { color: color.warning },
  dirMark: {
    height: space.s1_5,
    width: space.s1_5,
    flexShrink: 0,
    backgroundColor: color.warning,
  },
});

interface TreeContext {
  cwd?: string;
  focusedPath: string | null;
  onRowRemoved: (row: HTMLElement) => void;
  onEntryReady: (row: HTMLElement, fallback: boolean) => void;
  changes: ReadonlyMap<string, WorkspaceFileChange>;
  onFocusPath: (path: string) => void;
  onSelectFile: (path: string) => void;
  onReveal: (path: string) => void;
}

function changedBelow(changes: TreeContext["changes"], directory: string): boolean {
  const prefix = `${directory}/`;
  for (const path of changes.keys()) if (path.startsWith(prefix)) return true;
  return false;
}

function holds(entry: WorkspaceFileEntry, path: string | null): path is string {
  return path !== null && (path === entry.path || path.startsWith(`${entry.path}/`));
}

function TreeNode({
  entry,
  depth,
  cursor,
  tree,
  pos,
  size,
}: {
  entry: WorkspaceFileEntry;
  depth: number;
  cursor: string | null;
  tree: TreeContext;
  pos: number;
  size: number;
}) {
  const workspaceAccess = useWorkspace();

  const t = useT();
  const memory = useWorkspaceViewMemory();
  const isDir = entry.type === "dir";
  const expanded = isDir && memory.expandedDirs.includes(entry.path);
  const selected = !isDir && memory.lastFilePath === entry.path;
  const change = isDir ? undefined : tree.changes.get(entry.path);
  const dirChanged = isDir && changedBelow(tree.changes, entry.path);
  const statusId = useId();
  // The row and its group stay DOM siblings so the context menu trigger wraps only the row;
  // ownership restores the tree hierarchy while the group is mounted.
  const groupId = `${statusId}group`;
  const status = change
    ? change.change === "renamed"
      ? t("file.change.renamed", { path: change.previousPath ?? "" })
      : t(`file.change.${change.change}`)
    : dirChanged
      ? t("file.change.below")
      : undefined;
  const {
    data: children,
    isLoading,
    isFetching,
    error,
    refetch,
  } = useWorkspaceListFiles(expanded ? { cwd: tree.cwd, path: entry.path } : undefined);
  // A failed refresh of this directory's accepted children keeps them and their cursor in place.
  const retained = expanded && children !== undefined && children.length > 0;
  const shown = expanded && !isLoading && (error == null || retained) ? children : undefined;
  const childCursor =
    cursor !== entry.path && shown?.some((child) => holds(child, cursor)) ? cursor : null;
  const focusable = cursor !== null && childCursor === null;
  const standIn = focusable && entry.path !== tree.focusedPath;
  const { onRowRemoved, onEntryReady } = tree;
  const rowRef = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    const row = rowRef.current;
    return () => {
      if (row) onRowRemoved(row);
    };
  }, [onRowRemoved]);
  useLayoutEffect(() => {
    const row = rowRef.current;
    if (focusable && row) onEntryReady(row, standIn);
  }, [focusable, standIn, onEntryReady]);

  const activate = () =>
    isDir
      ? rememberWorkspaceView(workspaceAccess, {
          expandedDirs: expanded
            ? memory.expandedDirs.filter((dir) => dir !== entry.path)
            : [...memory.expandedDirs, entry.path],
        })
      : tree.onSelectFile(entry.path);

  const row = (
    <Tree.Item
      ref={rowRef}
      level={depth + 1}
      expanded={isDir ? expanded : undefined}
      selected={isDir ? undefined : selected}
      focusable={focusable}
      onActivate={activate}
      aria-owns={expanded ? groupId : undefined}
      aria-posinset={pos}
      aria-setsize={size}
      aria-label={entry.name}
      aria-describedby={status ? statusId : undefined}
      aria-current={selected ? "true" : undefined}
      title={change?.previousPath ? `${change.previousPath} → ${entry.path}` : entry.path}
      onFocus={() => tree.onFocusPath(entry.path)}
    >
      {isDir ? (
        <Icon
          name="chevron-down"
          size="xs"
          className={stylex.props(chevron.base, !expanded && chevron.shut).className}
        />
      ) : (
        <span {...stylex.props(ft.indent)} />
      )}
      <Icon
        name={isDir ? (expanded ? "folder-open" : "folder") : fileIconName(entry.path)}
        size="sm"
        className={stylex.props(vocab.hold, vocab.muted).className}
      />
      <span {...stylex.props(vocab.truncate, ft.name)}>
        {entry.name}
        {change?.previousPath && (
          <span {...stylex.props(vocab.muted)}> ← {change.previousPath}</span>
        )}
      </span>
      {change && (
        <span
          aria-hidden
          title={status}
          {...stylex.props(ft.mark, ft[change.change], typeStep.uiXs)}
        >
          {CHANGE_MARK[change.change]}
        </span>
      )}
      {dirChanged && <span aria-hidden title={status} {...stylex.props(ft.dirMark, corner.pill)} />}
      {status && (
        <span id={statusId} hidden>
          {status}
        </span>
      )}
    </Tree.Item>
  );

  return (
    <div role="none">
      <ContextMenu.Root>
        <ContextMenu.Trigger render={row} />
        <ContextMenu.Content aria-label={entry.path}>
          <ContextMenu.IconItem icon="copy" onSelect={() => void copyText(entry.path)}>
            {t("file.copyPath")}
          </ContextMenu.IconItem>
          {tree.cwd && workspaceAccess.local.available() && (
            <ContextMenu.IconItem icon="folder-open" onSelect={() => tree.onReveal(entry.path)}>
              {t("file.reveal")}
            </ContextMenu.IconItem>
          )}
        </ContextMenu.Content>
      </ContextMenu.Root>
      {expanded && (
        <Tree.Group id={groupId}>
          <DataView
            items={children}
            isLoading={isLoading}
            failure={retained ? undefined : error}
            onRetry={refetch}
            skeletonCount={1}
            error={{ title: t("dataView.error.title"), size: "compact" }}
            empty={{ icon: "folder", title: t("file.empty.title"), size: "compact" }}
          >
            {(rows) =>
              rows.map((child, index) => (
                <TreeNode
                  key={child.path}
                  entry={child}
                  depth={depth + 1}
                  cursor={holds(child, childCursor) ? childCursor : null}
                  tree={tree}
                  pos={index + 1}
                  size={rows.length}
                />
              ))
            }
          </DataView>
          {retained &&
            error != null &&
            (isFetching ? (
              <SkeletonList count={1} variant="compact" label={t("common.loading")} />
            ) : (
              <div role="status">
                <DataError
                  failure={error}
                  unsupported={{
                    icon: "folder",
                    sub: t("file.directoryRefreshError.sub", { path: entry.path }),
                    size: "compact",
                  }}
                  onRetry={refetch}
                  title={t("file.refreshError.title")}
                  sub={t("file.directoryRefreshError.sub", { path: entry.path })}
                  size="compact"
                />
              </div>
            ))}
        </Tree.Group>
      )}
    </div>
  );
}

export function FileTree({
  entries,
  cwd,
  ref,
  onSelectFile,
  onReveal,
}: {
  entries: WorkspaceFileEntry[];
  cwd?: string;
  ref?: Ref<HTMLDivElement>;
  onSelectFile: (path: string) => void;
  onReveal: (path: string) => void;
}) {
  const t = useT();
  const changes = useWorkingTreeFiles();
  const [focused, setFocused] = useState<string | null>(null);
  const focusLost = useRef(false);
  const onRowRemoved = useCallback((row: HTMLElement) => {
    if (row.ownerDocument.activeElement === row) focusLost.current = true;
  }, []);
  const onEntryReady = useCallback((row: HTMLElement, fallback: boolean) => {
    const reclaim = fallback && focusLost.current;
    focusLost.current = false;
    if (reclaim && row.ownerDocument.activeElement === row.ownerDocument.body) row.focus();
  }, []);
  const cursor = entries.some((entry) => holds(entry, focused))
    ? focused
    : (entries[0]?.path ?? null);
  const tree: TreeContext = {
    cwd,
    focusedPath: focused,
    onRowRemoved,
    onEntryReady,
    changes,
    onFocusPath: setFocused,
    onSelectFile,
    onReveal,
  };
  return (
    <Tree.Root ref={ref} aria-label={t("workspace.view.title.file")} {...stylex.props(ft.pad)}>
      {entries.map((entry, index) => (
        <TreeNode
          key={entry.path}
          entry={entry}
          depth={0}
          cursor={holds(entry, cursor) ? cursor : null}
          tree={tree}
          pos={index + 1}
          size={entries.length}
        />
      ))}
    </Tree.Root>
  );
}
