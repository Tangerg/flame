import * as stylex from "@stylexjs/stylex";
import { useId, useState } from "react";
import { copyText } from "@/lib/clipboard";
import { useT } from "@/lib/i18n";
import { ContextMenu, DataView, Icon, type IconName, Tree, chevron, vocab } from "@/ui";
import {
  rememberWorkspaceView,
  useWorkspaceViewMemory,
} from "@/plugins/builtin/workspace/application/navigation";
import {
  type WorkspaceFileChange,
  type WorkspaceFileEntry,
  useWorkspaceListFiles,
} from "@/plugins/builtin/workspace/application/workspaceQueries";
import { type FileKind, fileKind } from "@/plugins/builtin/workspace/application/fileKind";
import { useWorkingTreeFiles } from "@/plugins/builtin/workspace/application/workingTreeChanges";
import { revealWorkspacePath } from "../../adapters/desktopReveal";
import { color, corner, space, type as typeStep } from "@/styles/tokens.stylex";

const KIND_ICON: Record<FileKind, IconName> = {
  image: "image",
  document: "filetext",
  code: "code",
  file: "file",
};

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
  changes: ReadonlyMap<string, WorkspaceFileChange>;
  onFocusPath: (path: string) => void;
  onSelectFile: (path: string) => void;
}

function changedBelow(changes: TreeContext["changes"], directory: string): boolean {
  const prefix = `${directory}/`;
  for (const path of changes.keys()) if (path.startsWith(prefix)) return true;
  return false;
}

function TreeNode({
  entry,
  depth,
  tree,
}: {
  entry: WorkspaceFileEntry;
  depth: number;
  tree: TreeContext;
}) {
  const t = useT();
  const memory = useWorkspaceViewMemory();
  const isDir = entry.type === "dir";
  const expanded = isDir && memory.expandedDirs.includes(entry.path);
  const selected = !isDir && memory.lastFilePath === entry.path;
  const change = isDir ? undefined : tree.changes.get(entry.path);
  const dirChanged = isDir && changedBelow(tree.changes, entry.path);
  const statusId = useId();
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
    error,
    refetch,
  } = useWorkspaceListFiles(expanded ? { cwd: tree.cwd, path: entry.path } : undefined);

  const activate = () =>
    isDir
      ? rememberWorkspaceView({
          expandedDirs: expanded
            ? memory.expandedDirs.filter((dir) => dir !== entry.path)
            : [...memory.expandedDirs, entry.path],
        })
      : tree.onSelectFile(entry.path);

  const row = (
    <Tree.Item
      level={depth + 1}
      expanded={isDir ? expanded : undefined}
      selected={selected}
      focusable={entry.path === tree.focusedPath}
      onActivate={activate}
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
        name={isDir ? (expanded ? "folder-open" : "folder") : KIND_ICON[fileKind(entry.path)]}
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
        <ContextMenu.Content>
          <ContextMenu.IconItem icon="copy" onSelect={() => void copyText(entry.path)}>
            {t("file.copyPath")}
          </ContextMenu.IconItem>
          {tree.cwd && (
            <ContextMenu.IconItem
              icon="folder-open"
              onSelect={() => void revealWorkspacePath(tree.cwd!, entry.path)}
            >
              {t("file.reveal")}
            </ContextMenu.IconItem>
          )}
        </ContextMenu.Content>
      </ContextMenu.Root>
      {expanded && (
        <Tree.Group>
          <DataView
            items={children}
            isLoading={isLoading}
            failure={error}
            onRetry={refetch}
            skeletonCount={1}
            error={{ title: t("dataView.error.title"), size: "compact" }}
            empty={{ icon: "folder", title: t("file.empty.title"), size: "compact" }}
          >
            {(rows) =>
              rows.map((child) => (
                <TreeNode key={child.path} entry={child} depth={depth + 1} tree={tree} />
              ))
            }
          </DataView>
        </Tree.Group>
      )}
    </div>
  );
}

export function FileTree({
  entries,
  cwd,
  onSelectFile,
}: {
  entries: WorkspaceFileEntry[];
  cwd?: string;
  onSelectFile: (path: string) => void;
}) {
  const t = useT();
  const changes = useWorkingTreeFiles();
  const [focused, setFocused] = useState<string | null>(null);
  const focusedPath = focused ?? entries[0]?.path ?? null;
  const tree: TreeContext = {
    cwd,
    focusedPath,
    changes,
    onFocusPath: setFocused,
    onSelectFile,
  };
  return (
    <Tree.Root aria-label={t("workspace.view.title.file")} {...stylex.props(ft.pad)}>
      {entries.map((entry) => (
        <TreeNode key={entry.path} entry={entry} depth={0} tree={tree} />
      ))}
    </Tree.Root>
  );
}
