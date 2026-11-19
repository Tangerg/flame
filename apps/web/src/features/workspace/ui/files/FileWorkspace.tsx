import { useNotifications } from "@/features/notifications/public/notifier";
import { useWorkspace } from "@/features/workspace/public/workspace";
import { iconGlyph } from "@/ui/icons/index";
import * as stylex from "@stylexjs/stylex";
import {
  Fragment,
  useId,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type ClipboardEventHandler,
  type FormEvent,
  type KeyboardEventHandler,
  type MouseEvent,
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
  ContentVisibility,
  copyReaderSelection,
  EmptyState,
  IconButton,
  isImeKey,
  scrollReaderWithForeignSelection,
  scrollWithHorizontalArrow,
  selectAllInReader,
  TextField,
  useCopy,
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
  type ActiveSessionWorkspaceSelection,
} from "@/features/agent/public/session";
import { useWorkspaceReadFile } from "@/features/workspace/application/workspaceQueries";
import {
  closeWorkspaceFile,
  openWorkspaceFile,
  useWorkspaceFileViewer,
} from "@/features/workspace/public/navigation";
import { type WorkspaceFileViewer } from "@/features/workspace/application/navigationState";
import { FileTree } from "@/features/workspace/ui/files/FileTree";
import { useRendererChoice } from "@/features/plugins/public/renderers";
import { fileRendererCandidates } from "@/features/workspace/application/fileRenderer";
import { SystemMessage } from "@/ui/index";
import { useAsyncFeedback } from "@/lib/useAsyncFeedback";
const WINDOW_RADIUS = 200;

const fp = stylex.create({
  workspace: { display: "flex", flex: 1, flexDirection: "column", minWidth: 0, minHeight: 0 },
  hidden: { display: "none" },
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
  failure?: string;
  dismissFeedback(): void;
}

function LocalFileFeedback({
  actions,
  focusTarget,
}: {
  actions: LocalFileActions;
  focusTarget: RefObject<HTMLElement | null>;
}) {
  const t = useT();
  if (actions.failure === undefined) return null;
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
            )
              host?.focus({ preventScroll: true });
            actions.dismissFeedback();
          },
        }}
      >
        {actions.failure}
      </SystemMessage>
    </div>
  );
}

interface LineWindow {
  start?: number;
  end?: number;
  fromEnd?: boolean;
}

function initialWindow(line: number): LineWindow {
  return line > 0 ? { start: line, end: line + WINDOW_RADIUS } : {};
}

export function FileWorkspace() {
  const { messages } = useNotifications();
  const workspaceAccess = useWorkspace();

  const t = useT();
  const { feedback, run, reset } = useAsyncFeedback();
  const viewer = useWorkspaceFileViewer();
  const workspace = useActiveSessionWorkspace();
  const panelRef = useRef<HTMLDivElement>(null);
  const backRef = useRef<HTMLButtonElement>(null);
  const previousViewer = useRef(viewer);
  const restartFrom = useRef<WorkspaceFileViewer>(null);
  const focusFileOnReturn = useRef<((path: string) => void) | null>(null);
  const fileActions: LocalFileActions = {
    open(cwd, path) {
      void run(
        () => openLocalFile(messages, workspaceAccess, cwd, path),
        t("file.openFailed", { path }),
      );
    },
    reveal(cwd, path) {
      void run(
        () => revealLocalFile(messages, workspaceAccess, cwd, path),
        t("file.revealFailed", { path }),
      );
    },
    failure: feedback.state === "error" ? feedback.reason : undefined,
    dismissFeedback: reset,
  };
  useLayoutEffect(() => {
    const previous = previousViewer.current;
    previousViewer.current = viewer;
    const restarted = restartFrom.current;
    restartFrom.current = null;
    // The error action has already handed off focus inside the same reader.
    if (restarted && restarted === previous && viewer?.line === 0 && viewer.path === restarted.path)
      return;
    const panel = panelRef.current;
    if (!panel) return;
    const active = panel.ownerDocument.activeElement;
    if (active !== panel.ownerDocument.body && !panel.contains(active)) return;
    if (viewer) {
      backRef.current?.focus({ preventScroll: true });
    } else if (previous && active === panel.ownerDocument.body) {
      focusFileOnReturn.current?.(previous.path);
    }
  }, [viewer]);
  return (
    <div ref={panelRef} {...stylex.props(fp.workspace)}>
      <ContentVisibility visible={!viewer}>
        <div hidden={Boolean(viewer)} {...stylex.props(fp.workspace, viewer && fp.hidden)}>
          <FileBrowser
            workspace={workspace}
            focusFileOnReturn={focusFileOnReturn}
            fileActions={fileActions}
          />
        </div>
      </ContentVisibility>
      {viewer && (
        <FilePreview
          key={JSON.stringify([
            workspace.status,
            workspace.status === "ready" ? workspace.cwd : workspace.sessionId,
          ])}
          workspace={workspace}
          viewer={viewer}
          backRef={backRef}
          fileActions={fileActions}
          onOpenFromStart={() => {
            restartFrom.current = viewer;
            openWorkspaceFile(workspaceAccess, viewer.path);
          }}
        />
      )}
    </div>
  );
}

function FileBrowser({
  workspace,
  focusFileOnReturn,
  fileActions,
}: {
  workspace: ActiveSessionWorkspaceSelection;
  focusFileOnReturn: Ref<(path: string) => void>;
  fileActions: LocalFileActions;
}) {
  const workspaceAccess = useWorkspace();

  const t = useT();
  const cwd = resolvedWorkspaceCwd(workspace);
  const scrollPort = useRef<HTMLDivElement>(null);
  const treeRef = useRef<HTMLDivElement>(null);
  return (
    <WorkspaceViewLayout
      scrollRef={scrollPort}
      scrollInset="flush"
      notice={<LocalFileFeedback actions={fileActions} focusTarget={treeRef} />}
    >
      {isWorkspaceResolving(workspace) ? (
        <SkeletonList count={8} label={t("common.loading")} />
      ) : (
        <FileTree
          key={cwd}
          ref={treeRef}
          cwd={cwd}
          focusFileOnReturn={focusFileOnReturn}
          onSelectFile={openWorkspaceFile.bind(null, workspaceAccess)}
          onReveal={(path) => {
            if (cwd) fileActions.reveal(cwd, path);
          }}
        />
      )}
    </WorkspaceViewLayout>
  );
}

function coversWholeFile(file: { truncated?: boolean }, moreBefore: boolean, moreAfter: boolean) {
  return !file.truncated && !moreBefore && !moreAfter;
}

function FilePreviewMode({
  mode,
  onChange,
}: {
  mode: "source" | "rendered";
  onChange: (mode: "source" | "rendered") => void;
}) {
  const t = useT();
  return (
    <Segmented
      value={mode}
      onChange={onChange}
      ariaLabel={t("file.mode")}
      options={[
        { value: "rendered", label: t("file.mode.rendered") },
        { value: "source", label: t("file.mode.source") },
      ]}
    />
  );
}

function FilePreview({
  workspace,
  viewer,
  backRef,
  fileActions,
  onOpenFromStart,
}: {
  workspace: ActiveSessionWorkspaceSelection;
  viewer: WorkspaceFileViewer;
  backRef: Ref<HTMLButtonElement>;
  fileActions: LocalFileActions;
  onOpenFromStart: () => void;
}) {
  const workspaceAccess = useWorkspace();

  const t = useT();
  const copy = useCopy(viewer.path, t("file.copyPath"));
  const cwd = resolvedWorkspaceCwd(workspace);
  const targetLine = viewer.line;
  const initialLineWindow = useMemo(() => initialWindow(targetLine), [targetLine]);
  const [windowFor, setWindowFor] = useState<{ intent: object; window: LineWindow }>(() => ({
    intent: viewer,
    window: initialWindow(targetLine),
  }));
  const lineWindow = windowFor.intent === viewer ? windowFor.window : initialLineWindow;
  const setLineWindow = (next: LineWindow) => setWindowFor({ intent: viewer, window: next });

  const { data, isPlaceholderData, isPending, isFetching, error, refetch } = useWorkspaceReadFile(
    !isWorkspaceResolving(workspace)
      ? {
          cwd,
          path: viewer.path,
          ...(lineWindow.start !== undefined ? { startLine: lineWindow.start } : {}),
          ...(lineWindow.end !== undefined ? { endLine: lineWindow.end } : {}),
          ...(lineWindow.fromEnd ? { fromEnd: true } : {}),
        }
      : undefined,
  );

  const firstShown = data?.startLine ?? 1;
  const lastShown = data?.endLine ?? 0;
  const moreBefore = firstShown > 1;
  const laterCount = Math.min(WINDOW_RADIUS, Math.max(0, (data?.totalLines ?? 0) - lastShown));
  const moreAfter = laterCount > 0;

  const renderers = useExtensionPoint(WORKSPACE_FILE_RENDERER);
  const extension = fileExtension(viewer.path);
  const { renderer, control } = useRendererChoice(
    "file",
    extension,
    fileRendererCandidates(renderers, extension),
  );
  const wholeFile = !!data && coversWholeFile(data, moreBefore, moreAfter);
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

  const arrival = useRef<{ viewer: object; window: LineWindow } | null>(null);
  useLayoutEffect(() => {
    const host = scrollPort.current;
    if (!host || !data || isPlaceholderData) return;
    if (arrival.current?.viewer === viewer && arrival.current.window !== lineWindow) {
      host.scrollTop = 0;
    }
    arrival.current = { viewer, window: lineWindow };
  }, [data, isPlaceholderData, lineWindow, viewer]);

  const focusReaderFrom = (control: HTMLElement) => {
    if (control.ownerDocument.activeElement === control) {
      scrollPort.current?.focus({ preventScroll: true });
    }
  };
  const loadEarlier = (event: MouseEvent<HTMLElement>) => {
    focusReaderFrom(event.currentTarget);
    setLineWindow({
      start: Math.max(1, firstShown - WINDOW_RADIUS),
      end: firstShown - 1,
      fromEnd: true,
    });
  };
  const loadLater = (event: MouseEvent<HTMLElement>) => {
    focusReaderFrom(event.currentTarget);
    setLineWindow({ start: lastShown + 1, end: lastShown + laterCount });
  };

  // Set only while FileView presents the source, including as a renderer's fallback;
  // rendered content keeps the browser's formatted copy and editable fields stay native.
  const source = useRef<HTMLDivElement>(null);
  const selectionContentRef = useRef<Element | null>(null);
  const displayed = useRef<HTMLDivElement>(null);
  const onFileKeyDown: KeyboardEventHandler<HTMLDivElement> = (event) => {
    const sourceView = source.current;
    const sourceContent = sourceView ? selectionContentRef.current : null;
    const content = sourceContent ?? sourceView ?? displayed.current;
    if (content) selectAllInReader(event, content);
    if (
      (event.key === "ArrowLeft" || event.key === "ArrowRight") &&
      event.cancelable &&
      !isImeKey(event.nativeEvent) &&
      sourceContent instanceof HTMLElement &&
      sourceContent.scrollWidth > sourceContent.clientWidth
    ) {
      // The named viewport holds focus while Pierre's raw column pans.
      scrollWithHorizontalArrow(event, (delta) => {
        sourceContent.scrollBy({ left: delta, behavior: "instant" });
      });
    }
    scrollReaderWithForeignSelection(event, "both");
  };
  const copySource: ClipboardEventHandler<HTMLDivElement> = (event) => {
    if (source.current && data) copyReaderSelection(event, data.content, source.current);
  };

  const unsupported = workspaceAccess.errors.isUnsupportedFile(error);
  const canRender = wholeFile && !unsupported;
  const kind = fileKind(viewer.path);
  const icon = fileIconName(viewer.path);
  const { name } = splitFilePath(viewer.path);
  // A failed refresh of this query's own accepted content keeps its reader, and position, in place.
  const retained = data !== undefined && !isPlaceholderData && !unsupported;
  const rejectedInitialLine =
    !retained &&
    targetLine > 0 &&
    lineWindow.start === targetLine &&
    lineWindow.end === initialLineWindow.end &&
    !lineWindow.fromEnd &&
    workspaceAccess.errors.isInvalidReadRequest(error);
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
    data && !unsupported
      ? moreBefore || moreAfter
        ? t("file.range", { start: data.startLine, end: data.endLine, total: data.totalLines })
        : t("file.lines", { count: data.totalLines })
      : undefined;

  return (
    <WorkspaceViewLayout
      scrollRef={scrollPort}
      scrollInset="flush"
      scrollLabel={viewer.path}
      onScrollKeyDown={onFileKeyDown}
      onScrollCopy={copySource}
      notice={
        <>
          {refreshFeedback}
          {<LocalFileFeedback actions={fileActions} focusTarget={scrollPort} />}
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
        canRender && renderer ? <FilePreviewMode mode={mode} onChange={setMode} /> : undefined
      }
      actions={
        <>
          {canRender && control}
          {data && !unsupported && (
            <GoToLine
              key={viewer.path}
              onGo={(line) => openWorkspaceFile(workspaceAccess, viewer.path, line)}
            />
          )}
          <IconButton
            icon={iconGlyph("copy")}
            size="sm"
            aria-label={t("file.copyPath")}
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
        {/* Prefer directory boundaries before emergency wrapping without changing the path text. */}
        {viewer.path.split("/").map((component, index) => (
          <Fragment key={index}>
            {index > 0 && (
              <>
                /<wbr />
              </>
            )}
            {component}
          </Fragment>
        ))}
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
          isLoading={isPending || isWorkspaceResolving(workspace)}
          failure={retained ? undefined : error}
          onRetry={rejectedInitialLine ? onOpenFromStart : refetch}
          focusTarget={rejectedInitialLine ? scrollPort : undefined}
          skeletonCount={12}
          error={
            rejectedInitialLine
              ? {
                  title: t("file.error.requestedLine", { line: targetLine }),
                  sub: "",
                  retryLabel: t("file.openFromStart"),
                }
              : { title: t("file.error.title"), sub: t("file.error.sub") }
          }
        >
          {(items) => (
            <div ref={displayed}>
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
                      selectionContentRef={selectionContentRef}
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
                  selectionContentRef={selectionContentRef}
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
                    {t("file.loadLater", { count: laterCount })}
                  </TextButton>
                </div>
              )}
            </div>
          )}
        </DataView>
      )}
    </WorkspaceViewLayout>
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
