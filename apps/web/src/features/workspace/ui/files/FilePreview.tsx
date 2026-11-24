import { useWorkspace } from "@/features/workspace/public/workspace";
import { iconGlyph } from "@/ui/icons/index";
import * as stylex from "@stylexjs/stylex";
import { Fragment, useRef, useState, type Ref, type ReactNode, type RefObject } from "react";
import { fileExtension, splitFilePath } from "@/lib/path";
import { fileKind } from "@/lib/fileKind";
import { useExtensionPoint } from "@/extensions/selectors/extensions";
import { WORKSPACE_FILE_RENDERER } from "@/features/workspace/public/views";
import { DataError, DataView, SkeletonList } from "@/ui/index";
import { Segmented } from "@flame/ui";
import { Icon, fileIconName } from "@/ui/index";
import { Button, EmptyState, IconButton, useContentVisible, useCopy } from "@flame/ui";
import { face } from "@flame/ui/tokens.stylex";
import { useT } from "@/lib/i18n/index";
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
  FileReadingBody,
  GoToLine,
  useFileReaderInput,
  useRetiringControlFocus,
} from "@/features/workspace/ui/files/FileReader";
import {
  coversWholeFile,
  describeFileRead,
  fileLineSummary,
  useFileReadWindow,
} from "@/features/workspace/ui/files/fileReadWindow";

const styles = stylex.create({
  // The back chevron's stroke, not its button, starts the header's column edge (as
  // Disclosure's does); its hover fill bleeds like a row's.
  back: {
    marginInlineStart:
      "calc((var(--icon-sm) - var(--control-height-sm)) / 2 - var(--icon-sm) * 0.3125)",
  },
  directory: { display: "block", overflowWrap: "anywhere" },
  // The header never shrinks, so a long name is bounded here; the directory below completes it.
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
        action={{ label: t("common.retry"), variant: "outline", onClick: () => void onRetry() }}
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
      sub={t("file.unsupported.sub")}
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
  const wholeFile = !!data && coversWholeFile(data, moreBefore, moreAfter);
  const [mode, setMode] = usePreviewMode(displayedViewer);
  const showRendered = renderer !== undefined && wholeFile && mode === "rendered";

  const { onKeyDown, onCopy, ...reader } = useFileReaderInput(data?.content);

  const unsupported = workspaceAccess.errors.isUnsupportedFile(error);
  const retained = shown && !unsupported;
  const canRender = wholeFile && !unsupported;
  const readingControlRef = useRetiringControlFocus(scrollPort, visible && retained);

  const icon = fileIconName(viewer.path);
  const { name, directory } = splitFilePath(viewer.path);
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

  return (
    <WorkspaceViewLayout
      scrollRef={scrollPort}
      scrollInset="flush"
      scrollLabel={viewer.path}
      onScrollKeyDown={onKeyDown}
      onScrollCopy={onCopy}
      notice={
        <>
          <span role="status" className="sr-only">
            {pagePending ? t("common.loading") : undefined}
          </span>
          <div role="status">
            {refreshFailed && (
              <FileReadFeedback
                error={error}
                fetching={isFetching}
                pageErrorTitle={pageError?.title}
                jumpFailureTitle={jumpFailureTitle}
                icon={icon}
                retryRef={readingControlRef}
                focusTarget={scrollPort}
                onRetry={refetch}
              />
            )}
          </div>
          <LocalFileFeedback actions={fileActions} focusTarget={scrollPort} />
        </>
      }
      navigation={
        <IconButton
          ref={backRef}
          icon={iconGlyph("chevron-left")}
          size="sm"
          aria-label={t("file.backToFiles")}
          styles={styles.back}
          data-chrome-focus=""
          onClick={closeWorkspaceFile.bind(null, workspaceAccess)}
        />
      }
      icon={<Icon name={icon} size="sm" />}
      identity={
        <span title={viewer.path} {...stylex.props(styles.identityName)}>
          {name}
        </span>
      }
      sub={
        directory || readable ? (
          <>
            {directory && <FileDirectory directory={directory} />}
            {readable && fileLineSummary(t, data, moreBefore || moreAfter)}
            <FileJumpStatus label={jumpLabel} rejected={rejectedJump} />
          </>
        ) : undefined
      }
      controls={
        canRender && renderer !== undefined ? (
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
          failure={retained ? undefined : error}
          onRetry={rejectedInitialLine ? onOpenFromStart : refetch}
          focusTarget={pageError || rejectedInitialLine ? scrollPort : undefined}
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
              paging={showRendered ? undefined : { ...paging, failed: pageError !== undefined }}
            />
          )}
        </DataView>
      )}
    </WorkspaceViewLayout>
  );
}
