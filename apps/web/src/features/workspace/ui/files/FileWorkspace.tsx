import { useWorkspace } from "@/features/workspace/public/workspace";
import { iconGlyph } from "@/ui/icons/index";
import * as stylex from "@stylexjs/stylex";
import {
  Activity,
  Component,
  useCallback,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type ClipboardEventHandler,
  type FormEvent,
  type KeyboardEventHandler,
  type MouseEvent,
  type ReactNode,
  type Ref,
  type RefObject,
} from "react";
import { fileExtension, splitFilePath } from "@/lib/path";
import { fileKind } from "@/lib/fileKind";
import { useExtensionPoint } from "@/extensions/selectors/extensions";
import { PluginBoundary } from "@/extensions/host/PluginBoundary";
import { WORKSPACE_FILE_RENDERER } from "@/features/workspace/public/views";
import { DataError, DataView, SkeletonList } from "@/ui/index";
import { Popover } from "@/ui/index";
import { Segmented } from "@flame/ui";
import { TextButton } from "@/ui/index";
import { Icon, fileIconName } from "@/ui/index";
import {
  Button,
  copyWholeReader,
  EmptyState,
  IconButton,
  isImeKey,
  selectAllInReader,
  TextField,
  useCopyFeedback,
  vocab,
} from "@flame/ui";
import { color, face, space, type as typeStep } from "@flame/ui/tokens.stylex";

import { openLocalFile, revealLocalFile } from "@/features/workspace/application/localFileActions";

import { useT } from "@/lib/i18n/index";
import { FileView } from "@/features/workspace/ui/files/FileView";
import { WorkspaceViewLayout } from "@/features/workspace/ui/WorkspaceViewLayout";
import {
  isWorkspaceResolving,
  resolvedWorkspaceCwd,
  useActiveSessionWorkspace,
} from "@/features/agent/public/session";
import {
  useWorkspaceListFiles,
  useWorkspaceReadFile,
} from "@/features/workspace/application/workspaceQueries";
import {
  closeWorkspaceFile,
  openWorkspaceFile,
  useWorkspaceFileViewer,
} from "@/features/workspace/public/navigation";
import { type WorkspaceFileViewer } from "@/features/workspace/application/navigationState";
import { FileTree } from "@/features/workspace/ui/files/FileTree";
import { selectFileRenderer } from "@/features/workspace/application/fileRenderer";
import { type ViewHeaderProps } from "@/features/workspace/ui/ViewHeader";
import { SystemMessage } from "@/ui/index";
import { useAsyncFeedback } from "@/lib/useAsyncFeedback";
const WINDOW_RADIUS = 200;

const fp = stylex.create({
  workspace: { display: "flex", flex: 1, flexDirection: "column", minWidth: 0, minHeight: 0 },
  identity: { display: "flex", alignItems: "center", gap: space.s1_5, minWidth: 0 },
  identityIcon: { flexShrink: 0, color: color.fgFaint },
  // The header never shrinks, so a long name is bounded here; the reader shows the full path.
  identityName: {
    display: "-webkit-box",
    WebkitBoxOrient: "vertical",
    WebkitLineClamp: 2,
    overflow: "hidden",
    minWidth: 0,
    overflowWrap: "anywhere",
  },
  path: {
    margin: 0,
    paddingInline: space.s3,
    paddingBlockStart: space.s2,
    overflowWrap: "anywhere",
  },
  edge: { display: "flex", justifyContent: "center", paddingBlock: space.s1_5 },
  goto: {
    display: "flex",
    width: "220px",
    flexDirection: "column",
    gap: space.s2,
    padding: space.s2,
  },
  gotoRow: { display: "flex", flexWrap: "wrap", alignItems: "center", gap: space.s2 },
  gotoField: { flexGrow: 1, flexBasis: "6rem", minWidth: 0 },
  gotoError: { margin: 0, color: color.negative, overflowWrap: "anywhere" },
  feedback: { flexShrink: 0, maxHeight: "40dvh", overflowY: "auto", overscrollBehavior: "contain" },
});

interface LocalFileActions {
  open: (cwd: string, path: string) => void;
  reveal: (cwd: string, path: string) => void;
  feedback: (focusTarget: RefObject<HTMLElement | null>) => ReactNode;
}

interface LineWindow {
  start?: number;
  end?: number;
}

function initialWindow(line: number): LineWindow {
  return line > 0 ? { start: Math.max(1, line - WINDOW_RADIUS), end: line + WINDOW_RADIUS } : {};
}

export function FileWorkspace() {
  const workspaceAccess = useWorkspace();

  const t = useT();
  const { feedback, run, reset } = useAsyncFeedback();
  const viewer = useWorkspaceFileViewer();
  const panelRef = useRef<HTMLDivElement>(null);
  const backRef = useRef<HTMLButtonElement>(null);
  const previousViewer = useRef(viewer);
  const returnFocus = useRef(false);
  const fileActions: LocalFileActions = {
    open(cwd, path) {
      void run(() => openLocalFile(workspaceAccess, cwd, path), t("file.openFailed", { path }));
    },
    reveal(cwd, path) {
      void run(() => revealLocalFile(workspaceAccess, cwd, path), t("file.revealFailed", { path }));
    },
    feedback(focusTarget) {
      if (feedback.state !== "error") return undefined;
      return (
        <div {...stylex.props(fp.feedback)}>
          <SystemMessage
            variant="error"
            data-slot="local-file-feedback"
            action={{
              label: t("common.close"),
              onClick: () => {
                const host = focusTarget.current;
                const active = host?.ownerDocument.activeElement;
                if (
                  active instanceof HTMLElement &&
                  active.closest('[data-slot="local-file-feedback"]')
                ) {
                  host?.focus({ preventScroll: true });
                }
                reset();
              },
            }}
          >
            {feedback.reason}
          </SystemMessage>
        </div>
      );
    },
  };
  const restoreFocus = useCallback((item: HTMLElement) => {
    if (!returnFocus.current) return;
    returnFocus.current = false;
    if (item.ownerDocument.activeElement === item.ownerDocument.body) item.focus();
  }, []);
  useLayoutEffect(() => {
    const previous = previousViewer.current;
    previousViewer.current = viewer;
    const panel = panelRef.current;
    if (!panel) return;
    returnFocus.current = false;
    const active = panel.ownerDocument.activeElement;
    if (active !== panel.ownerDocument.body && !panel.contains(active)) return;
    if (viewer) {
      backRef.current?.focus({ preventScroll: true });
    } else if (previous) {
      returnFocus.current = true;
      const item = panel.querySelector<HTMLElement>('[role="treeitem"][tabindex="0"]');
      if (item) restoreFocus(item);
    }
  }, [viewer, restoreFocus]);
  return (
    <div ref={panelRef} {...stylex.props(fp.workspace)}>
      <Activity mode={viewer ? "hidden" : "visible"}>
        <FileBrowser restoreFocus={restoreFocus} fileActions={fileActions} />
      </Activity>
      {viewer && <FilePreview viewer={viewer} backRef={backRef} fileActions={fileActions} />}
    </div>
  );
}

interface FileContentLayoutProps extends ViewHeaderProps {
  viewport: RefObject<HTMLDivElement | null>;
  scrollLabel?: string;
  onScrollKeyDown?: KeyboardEventHandler<HTMLDivElement>;
  onScrollCopy?: ClipboardEventHandler<HTMLDivElement>;
  notice?: ReactNode;
  children: ReactNode;
}

function visibleInViewport(viewport: HTMLElement, row: HTMLElement): boolean {
  const bounds = viewport.getBoundingClientRect();
  const rect = row.getBoundingClientRect();
  return bounds.height > 0 && rect.top >= bounds.top && rect.bottom <= bounds.bottom;
}

// A pre-mutation snapshot distinguishes layout displacement from a user who scrolled away.
class FileContentLayout extends Component<FileContentLayoutProps, unknown, HTMLElement | null> {
  override getSnapshotBeforeUpdate(): HTMLElement | null {
    const viewport = this.props.viewport.current;
    const active = viewport?.ownerDocument.activeElement;
    return viewport &&
      active instanceof HTMLElement &&
      viewport.contains(active) &&
      visibleInViewport(viewport, active)
      ? active
      : null;
  }

  override componentDidUpdate(
    _previousProps: FileContentLayoutProps,
    _previousState: unknown,
    row: HTMLElement | null,
  ) {
    const viewport = this.props.viewport.current;
    if (
      row &&
      viewport &&
      row.ownerDocument.activeElement === row &&
      viewport.contains(row) &&
      !visibleInViewport(viewport, row)
    ) {
      row.scrollIntoView({ block: "nearest", inline: "nearest" });
    }
  }

  override render() {
    const { viewport, ...layout } = this.props;
    return <WorkspaceViewLayout {...layout} scrollInset="flush" scrollRef={viewport} />;
  }
}

function FileBrowser({
  restoreFocus,
  fileActions,
}: {
  restoreFocus: (item: HTMLElement) => void;
  fileActions: LocalFileActions;
}) {
  const workspaceAccess = useWorkspace();

  const t = useT();
  const workspace = useActiveSessionWorkspace();
  const cwd = resolvedWorkspaceCwd(workspace);
  const query = useWorkspaceListFiles(isWorkspaceResolving(workspace) ? undefined : { cwd });
  const scrollPort = useRef<HTMLDivElement>(null);
  const treeRef = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    const item = scrollPort.current?.querySelector<HTMLElement>('[role="treeitem"][tabindex="0"]');
    if (item) restoreFocus(item);
  }, [query.data, restoreFocus]);
  // A failed refresh of this query's accepted files keeps them, and the tree's cursor, in place.
  const retained = query.data !== undefined && query.data.length > 0;
  const refreshFeedback =
    retained && query.error != null ? (
      query.isFetching ? (
        <SkeletonList count={1} variant="compact" label={t("common.loading")} />
      ) : (
        <div role="status">
          <DataError
            failure={query.error}
            focusTarget={treeRef}
            unsupported={{ icon: "folder", sub: t("file.refreshError.sub"), size: "compact" }}
            onRetry={query.refetch}
            title={t("file.refreshError.title")}
            sub={t("file.refreshError.sub")}
            size="compact"
          />
        </div>
      )
    ) : undefined;
  return (
    <FileContentLayout
      viewport={scrollPort}
      notice={
        <>
          {refreshFeedback}
          {fileActions.feedback(treeRef)}
        </>
      }
    >
      <DataView
        items={query.data}
        isLoading={query.isLoading || isWorkspaceResolving(workspace)}
        failure={retained ? undefined : query.error}
        onRetry={query.refetch}
        unsupported={{ icon: "folder" }}
        skeletonCount={8}
        empty={{ icon: "folder", title: t("file.empty.title"), sub: t("file.empty.sub") }}
      >
        {(entries) => (
          <FileTree
            ref={treeRef}
            entries={entries}
            cwd={cwd}
            onSelectFile={openWorkspaceFile.bind(null, workspaceAccess)}
            onReveal={(path) => {
              if (cwd) fileActions.reveal(cwd, path);
            }}
          />
        )}
      </DataView>
    </FileContentLayout>
  );
}

function FilePreview({
  viewer,
  backRef,
  fileActions,
}: {
  viewer: WorkspaceFileViewer;
  backRef: Ref<HTMLButtonElement>;
  fileActions: LocalFileActions;
}) {
  const workspaceAccess = useWorkspace();

  const t = useT();
  const { copied, copy } = useCopyFeedback(viewer.path);
  const workspace = useActiveSessionWorkspace();
  const cwd = resolvedWorkspaceCwd(workspace);
  const targetLine = viewer.line;
  const [windowFor, setWindowFor] = useState<{ intent: object; window: LineWindow }>(() => ({
    intent: viewer,
    window: initialWindow(targetLine),
  }));
  const lineWindow = windowFor.intent === viewer ? windowFor.window : initialWindow(targetLine);
  const setLineWindow = (next: LineWindow) => setWindowFor({ intent: viewer, window: next });

  const { data, isPlaceholderData, isLoading, isFetching, error, refetch } = useWorkspaceReadFile(
    !isWorkspaceResolving(workspace)
      ? {
          cwd,
          path: viewer.path,
          ...(lineWindow.start !== undefined ? { startLine: lineWindow.start } : {}),
          ...(lineWindow.end !== undefined ? { endLine: lineWindow.end } : {}),
        }
      : undefined,
  );

  const shownLines = data ? data.content.split("\n").length : 0;
  const firstShown = data?.startLine ?? 1;
  const lastShown = firstShown + shownLines - 1;
  const moreBefore = firstShown > 1;
  const moreAfter = data !== undefined && lastShown < data.totalLines;

  const renderers = useExtensionPoint(WORKSPACE_FILE_RENDERER);
  const renderer = selectFileRenderer(renderers, fileExtension(viewer.path));
  const wholeFile = data !== undefined && !data.truncated && !moreBefore && !moreAfter;
  const [modeFor, setModeFor] = useState<{ intent: object; mode: "rendered" | "source" }>(() => ({
    intent: viewer,
    mode: targetLine > 0 ? "source" : "rendered",
  }));
  const mode = modeFor.intent === viewer ? modeFor.mode : targetLine > 0 ? "source" : "rendered";
  const setMode = (mode: "rendered" | "source") => setModeFor({ intent: viewer, mode });
  const showRendered = renderer !== undefined && wholeFile && mode === "rendered";

  const scrollPort = useRef<HTMLDivElement>(null);
  // A default arrival starts at the top like its initial window; FileView places line targets.
  useLayoutEffect(() => {
    const host = scrollPort.current;
    if (host && viewer.line <= 0) host.scrollTop = 0;
  }, [viewer]);

  // Bound to the window it requested; any later window or navigation retires it.
  const anchor = useRef<{ height: number; window: LineWindow } | null>(null);
  useLayoutEffect(() => {
    const host = scrollPort.current;
    if (!anchor.current || !host) return;
    if (anchor.current.window !== lineWindow) {
      anchor.current = null;
      return;
    }
    if (isPlaceholderData) return;
    host.scrollTop += host.scrollHeight - anchor.current.height;
    anchor.current = null;
  }, [data, isPlaceholderData, lineWindow]);

  const focusReaderFrom = (control: HTMLElement) => {
    if (control.ownerDocument.activeElement === control) {
      scrollPort.current?.focus({ preventScroll: true });
    }
  };
  const loadEarlier = (event: MouseEvent<HTMLElement>) => {
    focusReaderFrom(event.currentTarget);
    const host = scrollPort.current;
    const next = { start: Math.max(1, firstShown - WINDOW_RADIUS), end: lastShown };
    anchor.current = host ? { height: host.scrollHeight, window: next } : null;
    setLineWindow(next);
  };
  const loadLater = (event: MouseEvent<HTMLElement>) => {
    focusReaderFrom(event.currentTarget);
    setLineWindow({ start: firstShown, end: lastShown + WINDOW_RADIUS });
  };

  // Set only while FileView presents the source, including as a renderer's fallback;
  // rendered content and editable fields keep their native selection and copy.
  const source = useRef<HTMLDivElement>(null);
  const selectSource: KeyboardEventHandler<HTMLDivElement> = (event) => {
    if (source.current) selectAllInReader(event, source.current);
  };
  const copySource: ClipboardEventHandler<HTMLDivElement> = (event) => {
    if (source.current && data) copyWholeReader(event, data.content, source.current);
  };

  const unsupported = workspaceAccess.errors.isUnsupportedFile(error);
  const kind = fileKind(viewer.path);
  const icon = fileIconName(viewer.path);
  const { name } = splitFilePath(viewer.path);
  // A failed refresh of this query's own accepted content keeps its reader, and position, in place.
  const retained = data !== undefined && !isPlaceholderData && !unsupported;
  const refreshFeedback =
    retained && error != null ? (
      isFetching ? (
        <SkeletonList count={1} variant="compact" label={t("common.loading")} />
      ) : (
        <div role="status">
          <DataError
            failure={error}
            focusTarget={scrollPort}
            unsupported={{ icon, sub: t("file.readRefreshError.sub"), size: "compact" }}
            onRetry={refetch}
            title={t("file.readRefreshError.title")}
            sub={t("file.readRefreshError.sub")}
            size="compact"
          />
        </div>
      )
    ) : undefined;

  const sub =
    data && !unsupported ? (
      <span>
        {moreBefore || moreAfter
          ? t("file.range", { start: firstShown, end: lastShown, total: data.totalLines })
          : t("file.lines", { count: data.totalLines })}
        {data.truncated && ` · ${t("file.truncated")}`}
      </span>
    ) : undefined;

  return (
    <FileContentLayout
      viewport={scrollPort}
      scrollLabel={viewer.path}
      onScrollKeyDown={selectSource}
      onScrollCopy={copySource}
      notice={
        <>
          {refreshFeedback}
          {fileActions.feedback(scrollPort)}
        </>
      }
      navigation={
        <IconButton
          ref={backRef}
          icon={iconGlyph("chevron-left")}
          size="sm"
          aria-label={t("file.backToFiles")}
          data-chrome-focus=""
          onClick={closeWorkspaceFile.bind(null, workspaceAccess)}
        />
      }
      identity={
        <span title={viewer.path} {...stylex.props(fp.identity)}>
          <Icon name={icon} size="sm" className={stylex.props(fp.identityIcon).className} />
          <span {...stylex.props(fp.identityName)}>{name}</span>
        </span>
      }
      sub={sub}
      controls={
        renderer && wholeFile && !unsupported ? (
          <Segmented
            value={mode}
            onChange={setMode}
            ariaLabel={t("file.mode")}
            options={[
              { value: "rendered", label: t("file.mode.rendered") },
              { value: "source", label: t("file.mode.source") },
            ]}
          />
        ) : undefined
      }
      actions={
        <>
          {!unsupported && (
            <GoToLine onGo={(line) => openWorkspaceFile(workspaceAccess, viewer.path, line)} />
          )}
          <IconButton
            icon={iconGlyph(copied ? "check" : "copy")}
            size="sm"
            aria-label={t(copied ? "file.pathCopied" : "file.copyPath")}
            onClick={() => void copy()}
          />
          {cwd && workspaceAccess.local.available() && (
            <IconButton
              icon={iconGlyph("folder-open")}
              size="sm"
              aria-label={t("file.reveal")}
              onClick={() => fileActions.reveal(cwd, viewer.path)}
            />
          )}
        </>
      }
    >
      <p
        data-slot="file-path"
        dir="ltr"
        {...stylex.props(fp.path, face.mono, typeStep.uiXs, vocab.muted)}
      >
        {viewer.path}
      </p>
      {unsupported ? (
        <EmptyState
          icon={iconGlyph(icon)}
          title={t(kind === "image" ? "file.unsupported.image" : "file.unsupported.binary")}
          sub={t("file.unsupported.sub")}
          action={
            cwd &&
            workspaceAccess.local.available() && (
              <Button size="sm" onClick={() => fileActions.open(cwd, viewer.path)}>
                {t("file.open")}
              </Button>
            )
          }
        />
      ) : (
        <DataView
          items={data ? [data] : []}
          isLoading={isLoading || isWorkspaceResolving(workspace)}
          failure={retained ? undefined : error}
          onRetry={refetch}
          skeletonCount={12}
          error={{ title: t("file.error.title"), sub: t("file.error.sub") }}
        >
          {(items) => (
            <div>
              {moreBefore && !showRendered && (
                <div {...stylex.props(fp.edge)}>
                  <TextButton tone="accent" size="sm" onClick={loadEarlier}>
                    {t("file.loadEarlier", { count: Math.min(WINDOW_RADIUS, firstShown - 1) })}
                  </TextButton>
                </div>
              )}
              {showRendered && renderer ? (
                (() => {
                  const Renderer = renderer.component;
                  const recorded = (
                    <FileView
                      ref={source}
                      path={viewer.path}
                      content={items[0]!.content}
                      startLine={items[0]!.startLine}
                      targetLine={targetLine}
                      intent={isPlaceholderData ? null : viewer}
                    />
                  );
                  return (
                    <PluginBoundary
                      key={renderer.id}
                      plugin={renderer.id}
                      label={renderer.title}
                      fallback={recorded}
                    >
                      <Renderer cwd={cwd} path={viewer.path} {...items[0]!} fallback={recorded} />
                    </PluginBoundary>
                  );
                })()
              ) : (
                <FileView
                  ref={source}
                  path={viewer.path}
                  content={items[0]!.content}
                  startLine={items[0]!.startLine}
                  targetLine={targetLine}
                  intent={isPlaceholderData ? null : viewer}
                />
              )}
              {moreAfter && !showRendered && (
                <div {...stylex.props(fp.edge)}>
                  <TextButton tone="accent" size="sm" onClick={loadLater}>
                    {t("file.loadLater", { count: WINDOW_RADIUS })}
                  </TextButton>
                </div>
              )}
            </div>
          )}
        </DataView>
      )}
    </FileContentLayout>
  );
}

function lineNumber(value: string): number | null {
  const text = value.trim();
  if (!/^[0-9]+$/.test(text)) return null;
  const line = Number(text);
  return Number.isSafeInteger(line) && line > 0 ? line : null;
}

function GoToLine({ onGo }: { onGo: (line: number) => void }) {
  const t = useT();
  const errorId = useId();
  const field = useRef<HTMLInputElement>(null);
  const [open, setOpen] = useState(false);
  const [value, setValue] = useState("");
  const [attempted, setAttempted] = useState(false);
  const line = lineNumber(value);
  const invalid = attempted && line === null;
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (line === null) {
      setAttempted(true);
      field.current?.focus();
      return;
    }
    onGo(line);
    setOpen(false);
    setValue("");
    setAttempted(false);
  };
  return (
    <Popover.Root open={open} onOpenChange={setOpen}>
      <Popover.Trigger
        render={
          <IconButton icon={iconGlyph("crosshair")} size="sm" aria-label={t("file.goToLine")} />
        }
      />
      <Popover.Content align="end" sideOffset={6}>
        <form onSubmit={submit} {...stylex.props(fp.goto)}>
          <div {...stylex.props(fp.gotoRow)}>
            <TextField
              ref={field}
              inputMode="numeric"
              aria-label={t("file.goToLine")}
              aria-describedby={invalid ? errorId : undefined}
              invalid={invalid}
              placeholder={t("file.goToLine.placeholder")}
              value={value}
              onChange={(event) => setValue(event.target.value)}
              onKeyDown={(event) => {
                if (event.key !== "Enter") return;
                // Implicit submission would bypass the IME guard, so Enter always routes here.
                event.preventDefault();
                if (!isImeKey(event.nativeEvent)) event.currentTarget.form?.requestSubmit();
              }}
              styles={fp.gotoField}
            />
            <Button type="submit" variant="primary">
              {t("file.goToLine.submit")}
            </Button>
          </div>
          {invalid && (
            <p id={errorId} {...stylex.props(typeStep.uiSm, fp.gotoError)}>
              {t("file.goToLine.invalid")}
            </p>
          )}
        </form>
      </Popover.Content>
    </Popover.Root>
  );
}
