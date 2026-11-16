import { RemovalFocus, useContentVisible } from "@flame/ui";
import { useWorkspace } from "@/features/workspace/public/workspace";
import * as stylex from "@stylexjs/stylex";
import { type Ref, useCallback, useId, useLayoutEffect, useMemo, useRef } from "react";
import { useTree } from "@headless-tree/react";
import {
  type FeatureImplementation,
  type ItemInstance,
  type TreeInstance,
  hotkeysCoreFeature,
  selectionFeature,
  syncDataLoaderFeature,
} from "@headless-tree/core";
import { Button, copyText, isImeKey, vocab } from "@flame/ui";
import { useT } from "@/lib/i18n/index";
import {
  ContextMenu,
  DataError,
  DataView,
  SkeletonList,
  Icon,
  chevron,
  fileIconName,
} from "@/ui/index";
import {
  rememberWorkspaceView,
  useWorkspaceViewMemory,
} from "@/features/workspace/application/navigation";
import {
  type WorkspaceFileChange,
  type WorkspaceFileEntry,
} from "@/features/workspace/application/workspaceQueries";
import { useFileTreeData } from "@/features/workspace/application/fileTree";
import { useWorkingTreeFiles } from "@/features/workspace/application/workingTreeChanges";
import { color, corner, space, type as typeStep } from "@flame/ui/tokens.stylex";

// Visible rows already exist during a keyboard action. Commit focus now, so no deferred
// library poll can move it after another control or a retiring tree takes ownership.
const domFocusFeature: FeatureImplementation<WorkspaceFileEntry> = {
  key: "workspace-dom-focus",
  overwrites: ["tree"],
  treeInstance: {
    updateDomFocus: ({ tree }) => {
      const root = tree.getElement();
      const target = tree.getFocusedItem()?.getElement();
      if (root?.contains(root.ownerDocument.activeElement) && target?.isConnected) {
        target.focus({ preventScroll: true });
        target.scrollIntoView({ block: "nearest", inline: "nearest" });
      }
    },
  },
};

const CHANGE_MARK: Record<WorkspaceFileChange["change"], string> = {
  add: "A",
  mod: "M",
  del: "D",
  renamed: "R",
};

const ft = stylex.create({
  row: {
    whiteSpace: { default: "nowrap", ":is(:focus-within, [data-popup-open])": "normal" },
  },
  directory: {
    flexDirection: "column",
    alignItems: "stretch",
    justifyContent: "center",
    gap: 0,
  },
  header: { display: "flex", alignItems: "center", gap: space.s1_5, minWidth: 0 },
  feedback: { minWidth: 0, whiteSpace: "normal", userSelect: "text" },
  indent: { width: space.s3, flexShrink: 0 },
  pad: { paddingInline: space.s2, paddingBlock: space.s1_5 },
  name: { flex: 1, whiteSpace: "inherit", overflowWrap: "anywhere" },
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

type Model = ItemInstance<WorkspaceFileEntry>;
type Snapshot = ReturnType<typeof useFileTreeData>;
const entryId = (path: string) => `entry:${path}`;
const entryPath = (id: string) => id.slice("entry:".length);

function ownsTreeFocus(tree: TreeInstance<WorkspaceFileEntry>) {
  return tree.getFocusedItem()?.getElement() === tree.getElement()?.ownerDocument.activeElement;
}

function activate(item: Model) {
  if (!item.isFolder()) item.primaryAction();
  else if (item.isExpanded()) item.collapse();
  else item.expand();
}

function TreeNode({
  item,
  snapshot,
  changes,
  onReveal,
  revealable,
  onRemoved,
}: {
  item: Model;
  snapshot: Snapshot;
  changes: ReadonlyMap<string, WorkspaceFileChange>;
  onReveal(path: string): void;
  revealable: boolean;
  onRemoved(row: HTMLElement): void;
}) {
  const t = useT();
  const entry = item.getItemData();
  const isDir = item.isFolder();
  const expanded = item.isExpanded();
  const selected = !isDir && item.isSelected();
  const change = isDir ? undefined : changes.get(entry.path);
  const dirChanged = isDir && [...changes.keys()].some((path) => path.startsWith(`${entry.path}/`));
  const statusId = useId();
  const headerId = `${statusId}header`;
  const feedbackId = `${statusId}feedback`;
  const groupId = `${statusId}group`;
  const status = change
    ? change.change === "renamed"
      ? t("file.change.renamed", { path: change.previousPath ?? "" })
      : t(`file.change.${change.change}`)
    : dirChanged
      ? t("file.change.below")
      : undefined;
  const query = snapshot.directories.get(entry.path);
  const children = query?.data;
  const retained = children !== undefined;
  const feedbackTarget = {
    get current() {
      return item.getTree().getElement() ?? null;
    },
  };
  const bindRow = useCallback(
    (row: HTMLElement | null) => {
      item.registerElement(row);
      if (!row) return;
      return () => {
        onRemoved(row);
        item.registerElement(null);
      };
    },
    [item, onRemoved],
  );
  const label = (
    <>
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
          {" "}
          {CHANGE_MARK[change.change]}
        </span>
      )}
      {dirChanged && <span aria-hidden title={status} {...stylex.props(ft.dirMark, corner.pill)} />}
      {status && (
        <span id={statusId} hidden>
          {status}
        </span>
      )}
    </>
  );
  const row = (
    <Button
      {...item.getProps()}
      ref={bindRow}
      nativeButton={!isDir}
      render={isDir ? <div /> : undefined}
      shape="row"
      size="xs"
      active={selected}
      data-focus-proxy={isDir ? undefined : true}
      aria-label={
        isDir
          ? undefined
          : `${entry.name}${change?.previousPath ? ` ← ${change.previousPath}` : ""}${change ? ` ${CHANGE_MARK[change.change]}` : ""}`
      }
      aria-labelledby={isDir ? `${headerId}${expanded ? ` ${feedbackId}` : ""}` : undefined}
      aria-selected={isDir ? undefined : selected}
      aria-owns={expanded ? groupId : undefined}
      aria-describedby={status ? statusId : undefined}
      aria-current={selected ? "true" : undefined}
      aria-busy={isDir && expanded && query?.isFetching ? true : undefined}
      title={change?.previousPath ? `${change.previousPath} → ${entry.path}` : entry.path}
      onFocus={(event) => {
        if (event.target === event.currentTarget) item.setFocused();
      }}
      onKeyDownCapture={(event) => {
        if (event.target !== event.currentTarget) return;
        if (isImeKey(event.nativeEvent)) {
          event.stopPropagation();
          return;
        }
        if (
          (event.key !== "Enter" && event.key !== " ") ||
          event.altKey ||
          event.ctrlKey ||
          event.metaKey ||
          event.shiftKey
        )
          return;
        event.preventDefault();
        event.stopPropagation();
        activate(item);
      }}
      onKeyUpCapture={(event) => {
        if (!isDir || event.target !== event.currentTarget || event.key !== " ") return;
        // The row owns keydown activation; Base UI independently clicks non-native rows on Space keyup.
        event.preventDefault();
        event.stopPropagation();
      }}
      onClick={(event) => {
        if (
          isDir &&
          event.target !== event.currentTarget &&
          !event.currentTarget
            .querySelector('[data-slot="file-tree-header"]')
            ?.contains(event.target as Node)
        )
          return;
        item.setFocused();
        activate(item);
      }}
      styles={isDir ? ft.directory : undefined}
      className={stylex.props(ft.row).className}
      style={{ paddingInlineStart: `calc(${space.s1_5} + ${item.getItemMeta().level * 12}px)` }}
    >
      {isDir ? (
        <div id={headerId} data-slot="file-tree-header" {...stylex.props(ft.header)}>
          {label}
        </div>
      ) : (
        label
      )}
      {isDir && expanded && (
        <div id={feedbackId} {...stylex.props(ft.feedback)}>
          {" "}
          <DataView
            items={children}
            isLoading={!retained && (query === undefined || query.isPending)}
            failure={retained ? undefined : query?.error}
            focusTarget={feedbackTarget}
            onRetry={query?.refetch}
            skeletonCount={1}
            error={{ title: t("dataView.error.title"), size: "compact" }}
            empty={{ icon: "folder", title: t("file.empty.title"), size: "compact" }}
          >
            {() => null}
          </DataView>
          {retained &&
            query?.error != null &&
            (query.isFetching ? (
              <SkeletonList count={1} variant="compact" label={t("common.loading")} />
            ) : (
              <div role="status">
                <DataError
                  failure={query.error}
                  focusTarget={feedbackTarget}
                  unsupported={{
                    icon: "folder",
                    sub: t("file.directoryRefreshError.sub", { path: entry.path }),
                    size: "compact",
                  }}
                  onRetry={query.refetch}
                  title={t("file.refreshError.title")}
                  sub={t("file.directoryRefreshError.sub", { path: entry.path })}
                  size="compact"
                />
              </div>
            ))}
        </div>
      )}
    </Button>
  );
  return (
    <div role="presentation">
      <ContextMenu.Root>
        <ContextMenu.Trigger render={row} />
        <ContextMenu.Content aria-label={entry.path}>
          <ContextMenu.IconItem icon="copy" onSelect={() => void copyText(entry.path)}>
            {t("file.copyPath")}
          </ContextMenu.IconItem>
          {revealable && (
            <ContextMenu.IconItem icon="folder-open" onSelect={() => onReveal(entry.path)}>
              {t("file.reveal")}
            </ContextMenu.IconItem>
          )}
        </ContextMenu.Content>
      </ContextMenu.Root>
      {isDir && expanded && (
        <div role="group" id={groupId}>
          {children?.map((entry) => (
            <TreeNode
              key={entry.path}
              item={item.getTree().getItemInstance(entryId(entry.path))}
              snapshot={snapshot}
              changes={changes}
              onReveal={onReveal}
              revealable={revealable}
              onRemoved={onRemoved}
            />
          ))}
        </div>
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
  onSelectFile(path: string): void;
  onReveal(path: string): void;
}) {
  const workspace = useWorkspace();
  const t = useT();
  const changes = useWorkingTreeFiles();
  const memory = useWorkspaceViewMemory();
  const expanded = useMemo(() => memory.expandedDirs.map(entryId), [memory.expandedDirs]);
  const contentVisible = useContentVisible();
  const snapshot = useFileTreeData(entries, cwd, memory.expandedDirs, contentVisible);
  const rootId = `root:${useId()}`;
  const tree = useTree<WorkspaceFileEntry>({
    rootItemId: rootId,
    state: {
      expandedItems: expanded,
      selectedItems: memory.lastFilePath ? [entryId(memory.lastFilePath)] : [],
    },
    setExpandedItems: (next) =>
      rememberWorkspaceView(workspace, {
        expandedDirs: (typeof next === "function" ? next(expanded) : next).map(entryPath),
      }),
    getItemName: (item) => item.getItemData().name,
    isItemFolder: (item) => item.getItemData().type === "dir",
    onPrimaryAction: (item) => {
      if (!item.isFolder()) onSelectFile(item.getItemData().path);
    },
    dataLoader: {
      getItem: (id) => {
        const path = entryPath(id);
        const entry = snapshot.entries.get(path);
        if (!entry) throw new Error(`file tree item is not in the accepted snapshot: ${path}`);
        return entry;
      },
      getChildren: (id) =>
        (id === rootId ? entries : (snapshot.directories.get(entryPath(id))?.data ?? [])).map(
          (entry) => entryId(entry.path),
        ),
    },
    hotkeys: {
      focusNextItem: { hotkey: "ArrowDown", isEnabled: ownsTreeFocus },
      focusPreviousItem: { hotkey: "ArrowUp", isEnabled: ownsTreeFocus },
      expandOrDown: {
        hotkey: "ArrowRight",
        isEnabled: ownsTreeFocus,
        preventDefault: true,
        handler: (_event, tree) => {
          const item = tree.getFocusedItem();
          if (!item.isFolder()) return;
          if (!item.isExpanded()) {
            item.expand();
            return;
          }
          item.getChildren()[0]?.setFocused();
          tree.updateDomFocus();
        },
      },
      collapseOrUp: { hotkey: "ArrowLeft", isEnabled: ownsTreeFocus, preventDefault: true },
      focusFirstItem: { hotkey: "Home", isEnabled: ownsTreeFocus, preventDefault: true },
      focusLastItem: { hotkey: "End", isEnabled: ownsTreeFocus, preventDefault: true },
      // Selection is the workspace's activated file; tree browsing never changes it.
      toggleSelectedItem: { hotkey: "Control+Space", isEnabled: () => false },
      selectUpwards: { hotkey: "Shift+ArrowUp", isEnabled: () => false },
      selectDownwards: { hotkey: "Shift+ArrowDown", isEnabled: () => false },
      selectAll: { hotkey: "Control+KeyA", isEnabled: () => false },
    },
    features: [syncDataLoaderFeature, selectionFeature, hotkeysCoreFeature, domFocusFeature],
  });
  tree.scheduleRebuildTree();
  const visible = tree.getItems();
  const focused = tree.getState().focusedItem;
  const fallback =
    focused !== null && !visible.some((item) => item.getId() === focused)
      ? (visible.findLast((item) => focused.startsWith(`${item.getId()}/`)) ?? visible[0])
      : undefined;
  const removal = useRef<RemovalFocus | null>(null);
  const currentCwd = useRef(cwd);
  useLayoutEffect(() => {
    currentCwd.current = cwd;
  });
  const onRemoved = useCallback(
    (node: HTMLElement) => {
      const witness = RemovalFocus.capture(node, () => currentCwd.current === cwd);
      if (!witness) return;
      removal.current?.retire();
      removal.current = witness;
    },
    [cwd],
  );
  useLayoutEffect(() => {
    if (fallback) fallback.setFocused();
    const witness = removal.current;
    removal.current = null;
    witness?.restore(() => tree.getFocusedItem()?.getElement());
  });
  useLayoutEffect(
    () => () => {
      removal.current?.retire();
      removal.current = null;
    },
    [],
  );
  const bindRoot = useCallback(
    (node: HTMLDivElement | null) => {
      tree.registerElement(node);
      const release = typeof ref === "function" ? ref(node) : undefined;
      if (ref && typeof ref !== "function") ref.current = node;
      if (!node) return;
      return () => {
        tree.registerElement(null);
        if (typeof release === "function") release();
        else if (typeof ref === "function") ref(null);
        else if (ref) ref.current = null;
      };
    },
    [tree, ref],
  );
  return (
    <div
      {...tree.getContainerProps(t("workspace.view.title.file"))}
      ref={bindRoot}
      tabIndex={-1}
      onFocus={(event) => {
        if (event.target === event.currentTarget)
          tree.getFocusedItem()?.getElement()?.focus({ preventScroll: true });
      }}
      {...stylex.props(ft.pad)}
    >
      {entries.map((entry) => (
        <TreeNode
          key={entry.path}
          item={tree.getItemInstance(entryId(entry.path))}
          snapshot={snapshot}
          changes={changes}
          onReveal={onReveal}
          revealable={cwd !== undefined && workspace.local.available()}
          onRemoved={onRemoved}
        />
      ))}
    </div>
  );
}
