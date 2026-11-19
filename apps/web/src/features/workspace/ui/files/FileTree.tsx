import { useWorkingTreeFiles } from "@/features/workspace/application/workingTreeChanges";
import {
  useId,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent,
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
export function FileTree({
  cwd,
  ref,
  onSelectFile,
  onReveal,
}: {
  cwd?: string;
  ref?: RefObject<HTMLDivElement | null>;
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
  const onFileEnterCapture = (event: KeyboardEvent<HTMLDivElement>) => {
    const row = event.target;
    if (
      event.defaultPrevented ||
      !event.cancelable ||
      event.key !== "Enter" ||
      event.altKey ||
      event.ctrlKey ||
      event.metaKey ||
      event.shiftKey ||
      isImeKey(event.nativeEvent) ||
      !(row instanceof HTMLElement) ||
      row.ownerDocument.activeElement !== row ||
      !treeRef.current?.contains(row) ||
      !row.matches('[role="row"][data-file-path]:not([data-has-child-items])')
    )
      return;

    // WebKit can move focus to the Composer's retained selection before React Aria handles keyup.
    event.preventDefault();
  };
  return (
    <I18nProvider locale={locale}>
      <ContextMenu.Root>
        <div
          onKeyDownCapture={onFileEnterCapture}
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
