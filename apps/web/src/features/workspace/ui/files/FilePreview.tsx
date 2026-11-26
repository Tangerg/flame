import { useWorkspace } from "@/features/workspace/public/workspace";
import { iconGlyph } from "@/ui/icons/index";
import * as stylex from "@stylexjs/stylex";
import {
  Fragment,
  useRef,
  useState,
  type ComponentProps,
  type Ref,
  type ReactNode,
  type RefObject,
} from "react";
import { fileExtension, splitFilePath } from "@/lib/path";
import { fileKind } from "@/lib/fileKind";
import { useExtensionPoint } from "@/extensions/selectors/extensions";
import { WORKSPACE_FILE_RENDERER } from "@/features/workspace/public/views";
import { DataError, DataView, SkeletonList } from "@/ui/index";
import { Segmented } from "@flame/ui";
import { Icon, fileIconName } from "@/ui/index";
import {
  BackButton,
  Button,
  EmptyState,
  IconButton,
  Tooltip,
  useClampedText,
  useContentVisible,
  useCopy,
} from "@flame/ui";
import { face } from "@flame/ui/tokens.stylex";
import { useT, type Translate } from "@/lib/i18n/index";
import { WorkspaceViewLayout } from "@/features/workspace/ui/WorkspaceViewLayout";
import { type ActiveSessionWorkspaceSelection } from "@/features/agent/public/session";
import { closeWorkspaceFile } from "@/features/workspace/public/navigation";
import { type WorkspaceFileViewer } from "@/features/workspace/application/navigationState";
import { useRendererChoice } from "@/features/plugins/public/renderers";
import { fileRendererCandidates } from "@/features/workspace/application/fileRenderer";
import { SystemMessage } from "@/ui/index";
import { isUnsupportedMethod } from "@/lib/rpcErrors";
import {
  type LocalFileActions,
  LocalFileFeedback,
} from "@/features/workspace/ui/files/LocalFileFeedback";
import {
  FileJumpStatus,
  FilePageAction,
  FileReadingBody,
  GoToLine,
  useFileReaderInput,
  useRetiringControlFocus,
} from "@/features/workspace/ui/files/FileReader";
import {
  coversWholeFile,
  describeFileRead,
  fileRangeSummary,
  useFileReadWindow,
} from "@/features/workspace/ui/files/fileReadWindow";

const styles = stylex.create({
  directory: { display: "block", overflowWrap: "anywhere" },
  // A bounded basename leaves room for the reading controls in a narrow header.
  identityName: {
    display: "-webkit-box",
    WebkitBoxOrient: "vertical",
    WebkitLineClamp: 2,
    overflow: "hidden",
    minWidth: 0,
    overflowWrap: "anywhere",
  },
});

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

function FileDirectory({ directory }: { directory: string }) {
  return (
    <span dir="ltr" {...stylex.props(styles.directory, face.mono)}>
      {/* Prefer directory boundaries before emergency wrapping without changing the path text. */}
      {directory.split("/").map((component, index) => (
        <Fragment key={index}>
          {index > 0 && (
            <>
              /<wbr />
            </>
          )}
          {component}
        </Fragment>
      ))}
    </span>
  );
}

function FileReadFeedback({
  error,
  fetching,
  pageErrorTitle,
  jumpFailureTitle,
  icon,
  retryRef,
  focusTarget,
  onRetry,
}: {
  error: unknown;
  fetching: boolean;
  pageErrorTitle: string | undefined;
  jumpFailureTitle: string | undefined;
  icon: ReturnType<typeof fileIconName>;
  retryRef: Ref<HTMLDivElement>;
  focusTarget: RefObject<HTMLDivElement | null>;
  onRetry: () => unknown;
}) {
  const t = useT();
  if (fetching) return <SkeletonList count={1} variant="compact" label={t("common.loading")} />;
  if (pageErrorTitle !== undefined && !isUnsupportedMethod(error))
    return (
      <SystemMessage
        ref={retryRef}
        role="group"
        variant="error"
        hideIcon
        action={{ label: t("common.retry"), onClick: () => void onRetry() }}
      >
        {pageErrorTitle}
      </SystemMessage>
    );
  return (
    <DataError
      failure={error}
      focusTarget={focusTarget}
      unsupported={{ icon, sub: t("file.readRefreshError.sub"), size: "compact" }}
      onRetry={onRetry}
      title={jumpFailureTitle ?? t("file.readRefreshError.title")}
      sub={t("file.readRefreshError.sub")}
      size="compact"
    />
  );
}

type PreviewMode = "rendered" | "source";

// A line target opens the source; otherwise the rendered view leads. A choice
// belongs to the file intent it was made for.
function usePreviewMode(intent: WorkspaceFileViewer) {
  const defaultMode: PreviewMode = intent.line > 0 ? "source" : "rendered";
  const [choice, setChoice] = useState<{
    intent: WorkspaceFileViewer;
    mode: PreviewMode;
  }>(() => ({ intent, mode: defaultMode }));
  const mode = choice.intent === intent ? choice.mode : defaultMode;
  return [mode, (next: PreviewMode) => setChoice({ intent, mode: next })] as const;
}

function UnsupportedFile({
  path,
  cwd,
  fileActions,
}: {
  path: string;
  cwd: string | undefined;
  fileActions: LocalFileActions;
}) {
  const t = useT();
  const workspaceAccess = useWorkspace();
  return (
    <EmptyState
      icon={iconGlyph(fileIconName(path))}
      title={t(fileKind(path) === "image" ? "file.unsupported.image" : "file.unsupported.binary")}
      action={
        cwd &&
        workspaceAccess.local.available() && (
          <Button size="sm" onClick={() => fileActions.open(cwd, path)}>
            {t("file.open")}
          </Button>
        )
      }
    />
  );
}

function FilePreviewActions({
  path,
  cwd,
  rendererControl,
  onGoToLine,
  fileActions,
}: {
  path: string;
  cwd: string | undefined;
  rendererControl: ReactNode;
  onGoToLine: ((line: number) => void) | undefined;
  fileActions: LocalFileActions;
}) {
  const t = useT();
  const workspaceAccess = useWorkspace();
  const copy = useCopy(path, t("file.copyPath"));
  return (
    <>
      {rendererControl}
      {onGoToLine && <GoToLine key={path} onGo={onGoToLine} />}
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
          onClick={() => fileActions.reveal(cwd, path)}
        />
      )}
    </>
  );
}

// A renderer presents only a whole file the reader can read.
function renderedPresentation({
  rendererAvailable,
  wholeFile,
  mode,
  unsupported,
}: {
  rendererAvailable: boolean;
  wholeFile: boolean;
  mode: ReturnType<typeof usePreviewMode>[0];
  unsupported: boolean;
}) {
  const canRender = wholeFile && !unsupported;
  return {
    canRender,
    modeSwitch: canRender && rendererAvailable,
    showRendered: rendererAvailable && wholeFile && mode === "rendered",
  };
}

function fileSubline(
  t: Translate,
  {
    directory,
    lines,
    partial,
    jumpLabel,
    rejectedJump,
  }: {
    directory: string;
    lines: Parameters<typeof fileRangeSummary>[1] | undefined;
    partial: boolean;
    jumpLabel: string | undefined;
    rejectedJump: boolean;
  },
): ReactNode {
  if (!directory && !lines) return undefined;
  return (
    <>
      {directory && <FileDirectory directory={directory} />}
      {lines && partial && fileRangeSummary(t, lines)}
      <FileJumpStatus label={jumpLabel} rejected={rejectedJump} />
    </>
  );
}

// A rejected opening line retries from the start; a failed read or page keeps
// focus in the reader while it recovers.
function readRecovery({
  rejectedInitialLine,
  pageFailed,
  scrollPort,
  onOpenFromStart,
  refetch,
}: {
  rejectedInitialLine: boolean;
  pageFailed: boolean;
  scrollPort: RefObject<HTMLDivElement | null>;
  onOpenFromStart: () => void;
  refetch: () => unknown;
}) {
  return {
    onRetry: rejectedInitialLine ? onOpenFromStart : refetch,
    focusTarget: pageFailed || rejectedInitialLine ? scrollPort : undefined,
  };
}

// Page loading is announced only to assistive technology; a failed refresh
// keeps the read on screen with its own retry.
function FileReadNotice({
  pagePending,
  feedback,
  fileActions,
  focusTarget,
}: {
  pagePending: boolean;
  feedback: ComponentProps<typeof FileReadFeedback> | undefined;
  fileActions: LocalFileActions;
  focusTarget: RefObject<HTMLDivElement | null>;
}) {
  const t = useT();
  return (
    <>
      <span role="status" className="sr-only">
        {pagePending ? t("common.loading") : undefined}
      </span>
      <div role="status">{feedback && <FileReadFeedback {...feedback} />}</div>
      <LocalFileFeedback actions={fileActions} focusTarget={focusTarget} />
    </>
  );
}

export function FilePreview({
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
  const targetLine = viewer.line;
  const scrollPort = useRef<HTMLDivElement>(null);
  const {
    cwd,
    read: { isPlaceholderData, isFetching, error, refetch },
    page,
    data,
    loading,
    shown,
    retainedPrevious,
    awaitingPrevious,
    displayedRead,
    displayedViewer,
    retainedPending,
    pagePending,
    moreBefore,
    moreAfter,
    paging,
    goToLine,
  } = useFileReadWindow(workspace, viewer, scrollPort);

  const renderers = useExtensionPoint(WORKSPACE_FILE_RENDERER);
  const extension = fileExtension(viewer.path);
  const { renderer, control } = useRendererChoice(
    "file",
    extension,
    fileRendererCandidates(renderers, extension),
  );
  const [mode, setMode] = usePreviewMode(displayedViewer);

  const { onKeyDown, onCopy, ...reader } = useFileReaderInput(data?.content);

  const unsupported = workspaceAccess.errors.isUnsupportedFile(error);
  const retained = shown && !unsupported;
  const { canRender, modeSwitch, showRendered } = renderedPresentation({
    rendererAvailable: renderer !== undefined,
    wholeFile: !!data && coversWholeFile(data, moreBefore, moreAfter),
    mode,
    unsupported,
  });
  const readingControlRef = useRetiringControlFocus(scrollPort, visible && retained);

  const icon = fileIconName(viewer.path);
  const { name, directory } = splitFilePath(viewer.path);
  const nameRef = useRef<HTMLSpanElement>(null);
  const nameClamped = useClampedText(nameRef, viewer.path);
  const {
    refreshFailed,
    rejectedJump,
    rejectedInitialLine,
    jumpFailureTitle,
    jumpLabel,
    pageError,
    readError,
  } = describeFileRead(t, {
    page,
    targetLine,
    error,
    invalidRequest: workspaceAccess.errors.isInvalidReadRequest(error),
    retained,
    retainedPrevious,
    awaitingPrevious,
    retainedPending,
    placeholderFetching: isPlaceholderData && isFetching,
  });

  const readable = data !== undefined && !unsupported;
  const failure = retained ? undefined : error;
  const sourcePaging =
    readable && !loading && failure == null && !showRendered
      ? { ...paging, failed: pageError !== undefined }
      : undefined;

  return (
    <WorkspaceViewLayout
      scrollRef={scrollPort}
      scrollInset="flush"
      scrollLabel={viewer.path}
      onScrollKeyDown={onKeyDown}
      onScrollCopy={onCopy}
      beforeScroll={
        <>
          <FileReadNotice
            pagePending={pagePending}
            feedback={
              refreshFailed
                ? {
                    error,
                    fetching: isFetching,
                    pageErrorTitle: pageError?.title,
                    jumpFailureTitle,
                    icon,
                    retryRef: readingControlRef,
                    focusTarget: scrollPort,
                    onRetry: refetch,
                  }
                : undefined
            }
            fileActions={fileActions}
            focusTarget={scrollPort}
          />
          <FilePageAction edge="earlier" paging={sourcePaging} />
        </>
      }
      navigation={
        <BackButton
          ref={backRef}
          aria-label={t("file.backToFiles")}
          data-chrome-focus=""
          onClick={closeWorkspaceFile.bind(null, workspaceAccess)}
        />
      }
      icon={<Icon name={icon} size="sm" />}
      identity={
        <Tooltip label={<span dir="ltr">{viewer.path}</span>} disabled={!nameClamped}>
          <span
            ref={nameRef}
            role={nameClamped ? "group" : undefined}
            aria-label={nameClamped ? name : undefined}
            tabIndex={nameClamped ? 0 : undefined}
            {...stylex.props(styles.identityName)}
          >
            {name}
          </span>
        </Tooltip>
      }
      sub={fileSubline(t, {
        directory,
        lines: readable ? data : undefined,
        partial: moreBefore || moreAfter,
        jumpLabel,
        rejectedJump,
      })}
      controls={
        modeSwitch ? (
          <FilePreviewMode ref={readingControlRef} mode={mode} onChange={setMode} />
        ) : undefined
      }
      actions={
        <FilePreviewActions
          path={viewer.path}
          cwd={cwd}
          rendererControl={canRender ? control : undefined}
          onGoToLine={readable ? goToLine : undefined}
          fileActions={fileActions}
        />
      }
    >
      {unsupported ? (
        <UnsupportedFile path={viewer.path} cwd={cwd} fileActions={fileActions} />
      ) : (
        <DataView
          items={data ? [data] : []}
          isLoading={loading}
          failure={failure}
          {...readRecovery({
            rejectedInitialLine,
            pageFailed: pageError !== undefined,
            scrollPort,
            onOpenFromStart,
            refetch,
          })}
          skeletonCount={12}
          error={readError}
        >
          {(items) => (
            <FileReadingBody
              file={items[0]!}
              path={viewer.path}
              cwd={cwd}
              targetLine={displayedViewer.line}
              intent={displayedRead?.viewer ?? null}
              reader={reader}
              renderer={showRendered ? renderer : undefined}
              previewRef={readingControlRef}
              paging={sourcePaging}
            />
          )}
        </DataView>
      )}
    </WorkspaceViewLayout>
  );
}
