import { useNotifications } from "@/features/notifications/public/notifier";
import { useWorkspace } from "@/features/workspace/public/workspace";
import { iconGlyph } from "@/ui/icons/index";
import * as stylex from "@stylexjs/stylex";
import {
  Fragment,
  useCallback,
  useId,
  useLayoutEffect,
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
  handoffFocus,
  IconButton,
  isImeKey,
  scrollReaderWithForeignSelection,
  scrollWithHorizontalArrow,
  selectAllInReader,
  TextField,
  useContentVisible,
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
import {
  useCachedWorkspaceReadFile,
  useWorkspaceReadFile,
  type WorkspaceReadFileQuery,
} from "@/features/workspace/application/workspaceQueries";
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
import { isUnsupportedMethod } from "@/lib/rpcErrors";
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
  preview: { display: "contents" },
  edge: { display: "flex", justifyContent: "center", paddingBlock: space.s1_5 },
  jumpStatus: { display: "inline-block", maxWidth: "100%" },
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

type LineWindow = Pick<WorkspaceReadFileQuery, "startLine" | "endLine" | "fromEnd">;
type AdjacentLineWindow = LineWindow & { startLine: number; endLine: number };

function initialWindow(line: number): LineWindow {
  return line > 0 ? { startLine: line, endLine: line + WINDOW_RADIUS } : {};
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
  ref,
}: {
  mode: "source" | "rendered";
  onChange: (mode: "source" | "rendered") => void;
  ref: Ref<HTMLDivElement>;
}) {
  const t = useT();
  return (
    <Segmented
      ref={ref}
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
  const visible = useContentVisible();

  const t = useT();
  const copy = useCopy(viewer.path, t("file.copyPath"));
  const cwd = resolvedWorkspaceCwd(workspace);
  const targetLine = viewer.line;
  const [transitionFor, setTransitionFor] = useState<{
    viewer: WorkspaceFileViewer;
    window?: AdjacentLineWindow;
    previous?: { request: WorkspaceReadFileQuery; viewer: WorkspaceFileViewer };
  }>();
  const transition = transitionFor?.viewer === viewer ? transitionFor : undefined;
  const page = transition?.window;
  const lineWindow = page ?? initialWindow(targetLine);
  const resolving = isWorkspaceResolving(workspace);
  const request = resolving ? undefined : { cwd, path: viewer.path, ...lineWindow };
  const {
    data: response,
    isPlaceholderData,
    isPending,
    isFetching,
    error,
    refetch,
  } = useWorkspaceReadFile(request);
  const acceptedData = isPlaceholderData ? undefined : response;
  const previousRead = acceptedData === undefined ? transition?.previous : undefined;
  // The exact origin is authoritative even if the query observer still has a placeholder.
  const previousData = useCachedWorkspaceReadFile(previousRead?.request);
  const data = previousRead ? previousData : response;
  const displayedRead =
    previousData !== undefined ? previousRead : acceptedData && request && { request, viewer };
  const displayedViewer = displayedRead?.viewer ?? viewer;

  const retainedPending = previousData !== undefined && isFetching;
  const pagePending = page !== undefined && retainedPending;
  const pagingUnavailable = isPlaceholderData || retainedPending;
  const pageLoadingLabel = t("common.loading");
  const pageLoadingStatus = (
    <span aria-hidden="true" {...stylex.props(typeStep.uiMd, vocab.muted)}>
      {pageLoadingLabel}
    </span>
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
  const defaultMode = displayedViewer.line > 0 ? "source" : "rendered";
  const [modeFor, setModeFor] = useState<{
    intent: WorkspaceFileViewer;
    mode: "rendered" | "source";
  }>(() => ({
    intent: displayedViewer,
    mode: defaultMode,
  }));
  const mode = modeFor.intent === displayedViewer ? modeFor.mode : defaultMode;
  const setMode = (mode: "rendered" | "source") => setModeFor({ intent: displayedViewer, mode });
  const showRendered = renderer !== undefined && wholeFile && mode === "rendered";

  const scrollPort = useRef<HTMLDivElement>(null);
  const pageFailureNotice = useRef<HTMLDivElement>(null);
  // A default arrival starts at the top like its initial window; FileView places line targets.
  useLayoutEffect(() => {
    const host = scrollPort.current;
    if (host && viewer.line <= 0) host.scrollTop = 0;
  }, [viewer]);

  const arrivedPage = useRef<AdjacentLineWindow>(null);
  useLayoutEffect(() => {
    const host = scrollPort.current;
    if (!host || acceptedData === undefined) return;
    if (page && arrivedPage.current !== page) {
      host.scrollTop = 0;
      arrivedPage.current = page;
    }
    if (transition?.previous) {
      const settled = page ? { viewer, window: page } : undefined;
      setTransitionFor((current) => (current === transition ? settled : current));
    }
  }, [acceptedData, page, transition, viewer]);

  const loadPage = (control: HTMLElement, window: AdjacentLineWindow) => {
    if (!displayedRead) return;
    if (control.ownerDocument.activeElement === control) {
      scrollPort.current?.focus({ preventScroll: true });
    }
    setTransitionFor({ viewer, window, previous: displayedRead });
  };
  const loadEarlier = (event: MouseEvent<HTMLElement>) =>
    loadPage(event.currentTarget, {
      startLine: Math.max(1, firstShown - WINDOW_RADIUS),
      endLine: firstShown - 1,
      fromEnd: true,
    });
  const loadLater = (event: MouseEvent<HTMLElement>) =>
    loadPage(event.currentTarget, { startLine: lastShown + 1, endLine: lastShown + laterCount });
  const goToLine = (line: number) => {
    const nextViewer = openWorkspaceFile(workspaceAccess, viewer.path, line);
    setTransitionFor(displayedRead ? { viewer: nextViewer, previous: displayedRead } : undefined);
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
  const canSwitchMode = canRender && renderer !== undefined;
  const retiredFocusRoot = useRef<HTMLDivElement>(null);
  const readingControlRef = useCallback((root: HTMLDivElement | null) => {
    if (!root) return;
    if (retiredFocusRoot.current === root) retiredFocusRoot.current = null;
    return () => {
      // The retiring subtree still contains the focus owner before React removes it.
      if (root.contains(root.ownerDocument.activeElement)) retiredFocusRoot.current = root;
    };
  }, []);
  useLayoutEffect(() => {
    const root = retiredFocusRoot.current;
    retiredFocusRoot.current = null;
    const reader = scrollPort.current;
    if (
      visible &&
      acceptedData !== undefined &&
      !unsupported &&
      root?.isConnected === false &&
      reader?.isConnected &&
      reader.ownerDocument.activeElement === reader.ownerDocument.body
    ) {
      reader.focus({ preventScroll: true });
    }
  }, [acceptedData, canSwitchMode, showRendered, unsupported, visible]);

  const kind = fileKind(viewer.path);
  const icon = fileIconName(viewer.path);
  const { name } = splitFilePath(viewer.path);
  const retained = (acceptedData !== undefined || previousData !== undefined) && !unsupported;
  const invalidLine = !page && targetLine > 0 && workspaceAccess.errors.isInvalidReadRequest(error);
  const rejectedJump = invalidLine && previousData !== undefined;
  const rejectedInitialLine = invalidLine && !retained;
  const jumpFailureTitle =
    !page && previousData !== undefined && error != null
      ? t("file.error.requestedLine", { line: targetLine })
      : undefined;
  const jumpPending =
    !page && targetLine > 0 && (previousRead ? retainedPending : isPlaceholderData && isFetching);
  const jumpLabel = rejectedJump
    ? jumpFailureTitle
    : jumpPending
      ? t("file.goToLine.pending", { line: targetLine })
      : undefined;
  const pageError =
    page && previousRead && error != null
      ? {
          title: t("file.error.requestedRange", {
            start: page.startLine,
            end: page.endLine,
          }),
          sub: "",
          size: "compact" as const,
        }
      : undefined;
  const readFeedback =
    retained && error != null && !retainedPending && !rejectedJump ? (
      isFetching ? (
        <SkeletonList count={1} variant="compact" label={t("common.loading")} />
      ) : pageError && !isUnsupportedMethod(error) ? (
        <SystemMessage
          ref={pageFailureNotice}
          role="group"
          variant="error"
          hideIcon
          action={{
            label: t("common.retry"),
            variant: "outline",
            onClick: () => {
              if (pageFailureNotice.current)
                handoffFocus(pageFailureNotice.current, scrollPort.current);
              void refetch();
            },
          }}
        >
          {pageError.title}
        </SystemMessage>
      ) : (
        <DataError
          failure={error}
          focusTarget={scrollPort}
          unsupported={{ icon, sub: t("file.readRefreshError.sub"), size: "compact" }}
          onRetry={refetch}
          title={jumpFailureTitle ?? t("file.readRefreshError.title")}
          sub={t("file.readRefreshError.sub")}
          size="compact"
        />
      )
    ) : undefined;

  const lineSummary =
    data && !unsupported
      ? moreBefore || moreAfter
        ? t("file.range", { start: data.startLine, end: data.endLine, total: data.totalLines })
        : t("file.lines", { count: data.totalLines })
      : undefined;
  const sub = lineSummary && (
    <>
      {lineSummary}
      {jumpLabel && " "}
      <span
        role="status"
        {...stylex.props(fp.jumpStatus, rejectedJump ? vocab.negative : vocab.accent)}
      >
        {jumpLabel}
      </span>
    </>
  );

  return (
    <WorkspaceViewLayout
      scrollRef={scrollPort}
      scrollInset="flush"
      scrollLabel={viewer.path}
      onScrollKeyDown={onFileKeyDown}
      onScrollCopy={copySource}
      notice={
        <>
          <span role="status" className="sr-only">
            {pagePending ? pageLoadingLabel : undefined}
          </span>
          <div role="status">{readFeedback}</div>
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
        canSwitchMode ? (
          <FilePreviewMode ref={readingControlRef} mode={mode} onChange={setMode} />
        ) : undefined
      }
      actions={
        <>
          {canRender && control}
          {data && !unsupported && <GoToLine key={viewer.path} onGo={goToLine} />}
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
          isLoading={resolving || (data === undefined && (isPending || isPlaceholderData))}
          failure={retained ? undefined : error}
          onRetry={rejectedInitialLine ? onOpenFromStart : refetch}
          focusTarget={pageError || rejectedInitialLine ? scrollPort : undefined}
          skeletonCount={12}
          error={
            pageError ??
            (rejectedInitialLine
              ? {
                  title: t("file.error.requestedLine", { line: targetLine }),
                  sub: "",
                  retryLabel: t("file.openFromStart"),
                }
              : { title: t("file.error.title"), sub: t("file.error.sub") })
          }
        >
          {(items) => (
            <div ref={displayed}>
              {moreBefore && !showRendered && (
                <div {...stylex.props(fp.edge)}>
                  {pagePending && lineWindow.fromEnd ? (
                    pageLoadingStatus
                  ) : (
                    <TextButton
                      tone="accent"
                      size="sm"
                      disabled={
                        pagingUnavailable || (pageError !== undefined && !!lineWindow.fromEnd)
                      }
                      onClick={loadEarlier}
                    >
                      {t("file.loadEarlier", { count: Math.min(WINDOW_RADIUS, firstShown - 1) })}
                    </TextButton>
                  )}
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
                      targetLine={displayedViewer.line}
                      intent={displayedRead?.viewer ?? null}
                    />
                  );
                  return (
                    <div ref={readingControlRef} {...stylex.props(fp.preview)}>
                      <PluginBoundary
                        key={renderer.id}
                        plugin={renderer.id}
                        label={renderer.title}
                        fallback={recorded}
                      >
                        <Renderer cwd={cwd} path={viewer.path} {...items[0]!} fallback={recorded} />
                      </PluginBoundary>
                    </div>
                  );
                })()
              ) : (
                <FileView
                  ref={source}
                  selectionContentRef={selectionContentRef}
                  path={viewer.path}
                  content={items[0]!.content}
                  startLine={items[0]!.startLine}
                  targetLine={displayedViewer.line}
                  intent={displayedRead?.viewer ?? null}
                />
              )}
              {moreAfter && !showRendered && (
                <div {...stylex.props(fp.edge)}>
                  {pagePending && !lineWindow.fromEnd ? (
                    pageLoadingStatus
                  ) : (
                    <TextButton
                      tone="accent"
                      size="sm"
                      disabled={
                        pagingUnavailable || (pageError !== undefined && !lineWindow.fromEnd)
                      }
                      onClick={loadLater}
                    >
                      {t("file.loadLater", { count: laterCount })}
                    </TextButton>
                  )}
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
