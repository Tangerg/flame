import { useWorkingTreeFiles } from "@/features/workspace/application/workingTreeChanges";
import {
  useId,
  useImperativeHandle,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type ComponentPropsWithRef,
  type KeyboardEvent,
  type Ref,
  type RefObject,
  type ReactNode,
} from "react";
import * as stylex from "@stylexjs/stylex";
import {
  Tree,
  TreeItem,
  TreeItemContent,
  TreeLoadMoreItem,
  Collection,
  Virtualizer,
  ListLayout,
  I18nProvider,
  Button as TreeButton,
  type Key,
  type TreeRenderProps,
} from "react-aria-components";
import { Button, handoffFocus, isImeKey, useContentVisible, useCopy } from "@flame/ui";
import { ContextMenu, DataError, Icon, fileIconName, SkeletonList } from "@/ui/index";
import { useT, useLocale, localeToBcp47 } from "@/lib/i18n/index";
import { useWorkspace } from "@/features/workspace/public/workspace";
import {
  rememberWorkspaceView,
  useWorkspaceViewMemory,
} from "@/features/workspace/application/navigation";
import { useDirectory } from "@/features/workspace/application/fileTree";
import type { WorkspaceFileEntry } from "@/features/workspace/application/workspaceQueries";
import { space, color, radius, type as typeStep } from "@flame/ui/tokens.stylex";
const styles = stylex.create({
  tree: { minHeight: 0, height: "100%", overflow: "auto", padding: space.s2 },
  row: {
    display: "flex",
    alignItems: "center",
    gap: space.s1_5,
    minHeight: "2rem",
    paddingInline: space.s2,
    paddingBlock: space.s1,
    borderRadius: radius.sm,
    color: color.fg,
    outlineOffset: "-2px",
    backgroundColor: { default: "transparent", "[aria-current]": "var(--control-active-surface)" },
  },
  name: { flex: 1, overflowWrap: "anywhere" },
});
type Directory = ReturnType<typeof useDirectory>;
function More({ query }: { query: Directory }) {
  const t = useT();
  if (query.last?.complete && !query.isError) return null;
  const automatic = query.hasNextPage && !query.isError && !query.last?.coverage.budgetExhausted;
  return (
    <TreeLoadMoreItem
      isLoading={query.isPending || query.isFetchingNextPage}
      onLoadMore={
        automatic
          ? () => {
              void query.fetchNextPage();
            }
          : undefined
      }
    >
      <span role="status">{t("common.loading")}</span>
    </TreeLoadMoreItem>
  );
}

function DirectoryStatus({
  cwd,
  path,
  focusTarget,
}: {
  cwd?: string;
  path: string;
  focusTarget: RefObject<HTMLDivElement | null>;
}) {
  const visible = useContentVisible();
  const query = useDirectory(cwd, path, visible);
  const t = useT();
  let content: ReactNode;
  if (query.isError && (path !== "" || query.entries.length > 0)) {
    content = (
      <DataError
        failure={query.error}
        focusTarget={focusTarget}
        title={query.data ? t("file.refreshError.title") : undefined}
        sub={query.data ? t("file.refreshError.sub", { path: path || "." }) : undefined}
        onRetry={() => {
          void (query.isFetchNextPageError ? query.fetchNextPage() : query.refetch());
        }}
        size="compact"
      />
    );
  } else if (query.last?.coverage.budgetExhausted && query.hasNextPage) {
    content = (
      <Button
        onClick={(event) => {
          handoffFocus(event.currentTarget, focusTarget.current);
          void query.fetchNextPage();
        }}
      >
        {t("file.loadMore")}
      </Button>
    );
  } else if (path !== "" && query.last?.complete && query.entries.length === 0) {
    content = <span>{t("file.empty.title")}</span>;
  } else return null;
  return (
    <div role="status" aria-label={path || "."}>
      <strong>{path || "."}</strong>
      {content}
    </div>
  );
}

function Item({
  entry,
  cwd,
  expanded,
  ancestorsExpanded,
  currentPath,
  onSelectFile,
}: {
  id: string;
  ancestorsExpanded: boolean;
  currentPath: string | null;
  entry: WorkspaceFileEntry;
  cwd?: string;
  expanded: ReadonlySet<Key>;
  onSelectFile(path: string): void;
}) {
  const visible = useContentVisible();
  const t = useT();
  const changeId = useId();
  const directory = entry.type === "dir";
  const query = useDirectory(
    cwd,
    entry.path,
    directory && ancestorsExpanded && expanded.has(entry.path) && visible,
  );
  const changes = useWorkingTreeFiles();
  const change = changes.get(entry.path);
  return (
    <TreeItem
      id={entry.path}
      textValue={entry.name}
      aria-label={entry.name}
      aria-describedby={change ? changeId : undefined}
      data-file-path={entry.path}
      hasChildItems={directory}
      onAction={directory ? undefined : () => onSelectFile(entry.path)}
    >
      <TreeItemContent>
        {({ isExpanded, level }) => (
          <div
            aria-current={!directory && currentPath === entry.path ? "true" : undefined}
            aria-busy={directory && query.isFetching ? true : undefined}
            {...stylex.props(styles.row, typeStep.uiSm)}
            title={change?.previousPath ? `${change.previousPath} → ${entry.path}` : entry.path}
            style={{ paddingInlineStart: `calc(${level - 1} * 1rem)` }}
          >
            {directory && (
              <TreeButton slot="chevron">
                <Icon name={isExpanded ? "chevron-down" : "chevron-right"} size="xs" />
              </TreeButton>
            )}
            <Icon name={directory ? "folder" : fileIconName(entry.path)} size="sm" />
            <span {...stylex.props(styles.name)}>
              {entry.name}
              {change?.previousPath && <> ← {change.previousPath}</>}
            </span>
            {change && (
              <span id={changeId} hidden>
                {t(`file.change.${change.change}`, { path: change.previousPath ?? entry.path })}
              </span>
            )}
            {change && (
              <span aria-hidden="true">
                {change.change === "add"
                  ? "A"
                  : change.change === "del"
                    ? "D"
                    : change.change === "renamed"
                      ? "R"
                      : "M"}
              </span>
            )}
          </div>
        )}
      </TreeItemContent>
      {directory && (
        <Collection
          items={query.entries}
          dependencies={[expanded, ancestorsExpanded, currentPath, cwd, onSelectFile]}
        >
          {(child) => (
            <Item
              id={child.path}
              entry={child}
              cwd={cwd}
              expanded={expanded}
              ancestorsExpanded={ancestorsExpanded && expanded.has(entry.path)}
              currentPath={currentPath}
              onSelectFile={onSelectFile}
            />
          )}
        </Collection>
      )}
      {directory && <More query={query} />}
    </TreeItem>
  );
}
function hasFocusableFile(state: TreeRenderProps["state"], path: string): boolean {
  // The flattened collection excludes collapsed descendants but includes offscreen rows.
  return (
    !state.selectionManager.isDisabled(path) &&
    [...state.collection].some((item) => item.type === "item" && item.key === path)
  );
}

function FileTreeViewport({
  state,
  treeRef,
  focusFileOnReturn,
  ...props
}: ComponentPropsWithRef<"div"> & {
  state: TreeRenderProps["state"];
  treeRef: RefObject<HTMLDivElement | null>;
  focusFileOnReturn: Ref<(path: string) => void>;
}) {
  const visible = useContentVisible();
  const revealKey = useRef<Key | null>(null);
  const pendingPath = useRef<string | null>(null);
  useImperativeHandle(
    focusFileOnReturn,
    () => (path) => {
      pendingPath.current = null;
      const viewport = treeRef.current;
      if (!visible || !viewport) return;
      const document = viewport.ownerDocument;
      if (document.activeElement !== document.body) return;
      const { selectionManager } = state;
      const available = hasFocusableFile(state, path);
      if (available) selectionManager.setFocusedKey(path);
      revealKey.current = selectionManager.focusedKey;
      viewport.focus({ preventScroll: true });
      if (!available && document.activeElement === viewport) pendingPath.current = path;
      if (!viewport.contains(document.activeElement)) revealKey.current = null;
    },
    [state, treeRef, visible],
  );
  useLayoutEffect(() => {
    if (!visible) {
      pendingPath.current = null;
      revealKey.current = null;
      return;
    }
    const path = pendingPath.current;
    if (path === null) return;
    const viewport = treeRef.current;
    if (!viewport || viewport.ownerDocument.activeElement !== viewport) {
      pendingPath.current = null;
      return;
    }
    if (state.collection.size === 0) return;
    const { selectionManager } = state;
    // The empty tree can receive DOM focus before React Aria installs its focus handler.
    if (!selectionManager.isFocused) selectionManager.setFocused(true);
    if (!hasFocusableFile(state, path)) return;

    pendingPath.current = null;
    revealKey.current = path;
    selectionManager.setFocusedKey(path);
  }, [state, treeRef, visible]);

  return (
    <div
      {...props}
      onFocusCapture={(event) => {
        props.onFocusCapture?.(event);
        const { currentTarget: viewport, target: row } = event;
        if (row !== viewport && viewport.contains(row)) pendingPath.current = null;
        const key = revealKey.current;
        if (key === null || row === viewport) return;
        revealKey.current = null;
        if (
          !(row instanceof HTMLElement) ||
          !viewport.contains(row) ||
          row.dataset.filePath !== key
        )
          return;

        // React Aria persists and focuses the key. Reveal its actual row after it mounts.
        const bounds = row.getBoundingClientRect();
        const top = viewport.getBoundingClientRect().top + viewport.clientTop;
        const bottom = top + viewport.clientHeight;
        if (bounds.top < top) viewport.scrollTop += bounds.top - top;
        else if (bounds.bottom > bottom) viewport.scrollTop += bounds.bottom - bottom;
      }}
      onBlurCapture={(event) => {
        props.onBlurCapture?.(event);
        if (!event.currentTarget.contains(event.relatedTarget)) {
          pendingPath.current = null;
          revealKey.current = null;
        }
      }}
    />
  );
}

export function FileTree({
  cwd,
  ref,
  focusFileOnReturn,
  onSelectFile,
  onReveal,
}: {
  cwd?: string;
  ref?: RefObject<HTMLDivElement | null>;
  focusFileOnReturn: Ref<(path: string) => void>;
  onSelectFile(path: string): void;
  onReveal(path: string): void;
}) {
  const workspace = useWorkspace();
  const internalRef = useRef<HTMLDivElement>(null);
  const treeRef = ref ?? internalRef;
  const t = useT();
  const visible = useContentVisible();
  const locale = localeToBcp47(useLocale());
  const [menuPath, setMenuPath] = useState("");
  const copy = useCopy(menuPath, t("file.copyPath"));
  const memory = useWorkspaceViewMemory();
  const expanded = useMemo(() => new Set<Key>(memory.expandedDirs), [memory.expandedDirs]);
  const statusPaths = [...expanded].map(String).filter((path) => {
    const parts = path.split("/");
    return parts
      .slice(0, -1)
      .every((_, index) => expanded.has(parts.slice(0, index + 1).join("/")));
  });
  const query = useDirectory(cwd, "", visible);
  const empty: ReactNode = query.isPending ? (
    <SkeletonList count={8} label={t("common.loading")} />
  ) : query.isError ? (
    <DataError failure={query.error} focusTarget={treeRef} onRetry={() => query.refetch()} />
  ) : query.last?.complete ? (
    <span>{t("file.empty.title")}</span>
  ) : (
    <span>{t("common.loading")}</span>
  );
  const onTreeRowKeyDownCapture = (event: KeyboardEvent<HTMLDivElement>) => {
    const { key, target: row } = event;
    if (
      event.defaultPrevented ||
      !event.cancelable ||
      (key !== "Enter" && key !== "ArrowLeft" && key !== "ArrowRight") ||
      event.altKey ||
      event.ctrlKey ||
      event.metaKey ||
      event.shiftKey ||
      isImeKey(event.nativeEvent) ||
      !(row instanceof HTMLElement) ||
      row.ownerDocument.activeElement !== row ||
      !treeRef.current?.contains(row) ||
      !row.matches('[role="row"][data-file-path]') ||
      (key === "Enter" && row.hasAttribute("data-has-child-items"))
    )
      return;

    // Keep WebKit's retained Composer selection from receiving keys that React Aria owns.
    event.preventDefault();
  };
  return (
    <I18nProvider locale={locale}>
      <ContextMenu.Root>
        <div
          onKeyDownCapture={onTreeRowKeyDownCapture}
          style={{ display: "flex", flexDirection: "column", height: "100%", minHeight: 0 }}
        >
          <Virtualizer layout={ListLayout} shouldObserveItemSize>
            <ContextMenu.Trigger
              onContextMenu={(event) => {
                const row =
                  event.target instanceof Element
                    ? event.target.closest<HTMLElement>("[data-file-path]")
                    : null;
                if (!row || !event.currentTarget.contains(row)) {
                  event.preventBaseUIHandler();
                  return;
                }
                setMenuPath(row.dataset.filePath!);
              }}
              render={
                <Tree
                  ref={treeRef}
                  render={(props, { state }) => (
                    <FileTreeViewport
                      {...props}
                      state={state}
                      treeRef={treeRef}
                      focusFileOnReturn={focusFileOnReturn}
                    />
                  )}
                  aria-label={t("workspace.view.title.file")}
                  expandedKeys={expanded}
                  onExpandedChange={(keys) =>
                    rememberWorkspaceView(workspace, { expandedDirs: [...keys].map(String) })
                  }
                  selectionMode="none"
                  renderEmptyState={() => empty}
                  {...stylex.props(styles.tree)}
                >
                  <Collection
                    items={query.entries}
                    dependencies={[expanded, memory.lastFilePath, cwd, onSelectFile]}
                  >
                    {(entry) => (
                      <Item
                        id={entry.path}
                        entry={entry}
                        cwd={cwd}
                        expanded={expanded}
                        ancestorsExpanded={true}
                        currentPath={memory.lastFilePath}
                        onSelectFile={onSelectFile}
                      />
                    )}
                  </Collection>
                  {query.hasNextPage && <More query={query} />}
                </Tree>
              }
            />
          </Virtualizer>
          <div style={{ flexShrink: 0, maxHeight: "40vh", overflow: "auto" }}>
            <DirectoryStatus cwd={cwd} path="" focusTarget={treeRef} />
            {statusPaths.map((path) => (
              <DirectoryStatus key={path} cwd={cwd} path={path} focusTarget={treeRef} />
            ))}
          </div>
        </div>
        <ContextMenu.Content aria-label={menuPath}>
          <ContextMenu.IconItem icon="copy" onSelect={() => void copy()}>
            {t("file.copyPath")}
          </ContextMenu.IconItem>
          {cwd !== undefined && workspace.local.available() && (
            <ContextMenu.IconItem icon="folder-open" onSelect={() => onReveal(menuPath)}>
              {t("file.reveal")}
            </ContextMenu.IconItem>
          )}
        </ContextMenu.Content>
      </ContextMenu.Root>
    </I18nProvider>
  );
}
