import { useWorkspace } from "@/features/workspace/public/workspace";
import { useLayoutEffect, useRef, useState, type MouseEvent, type RefObject } from "react";
import { useT, type Translate } from "@/lib/i18n/index";
import {
  isWorkspaceResolving,
  resolvedWorkspaceCwd,
  type ActiveSessionWorkspaceSelection,
} from "@/features/agent/public/session";
import {
  useCachedWorkspaceReadFile,
  useWorkspaceReadFile,
  type WorkspaceReadFileQuery,
} from "@/features/workspace/application/workspaceQueries";
import { openWorkspaceFile } from "@/features/workspace/public/navigation";
import { type WorkspaceFileViewer } from "@/features/workspace/application/navigationState";

const WINDOW_RADIUS = 200;

type LineWindow = Pick<WorkspaceReadFileQuery, "startLine" | "endLine" | "fromEnd">;

type AdjacentLineWindow = LineWindow & { startLine: number; endLine: number };

export type FileReadPaging = ReturnType<typeof useFileReadWindow>["paging"];

function initialWindow(line: number): LineWindow {
  return line > 0 ? { startLine: line, endLine: line + WINDOW_RADIUS } : {};
}

// What the accepted read shows and how many lines each adjacent page may add.
function shownLines(data: { startLine: number; endLine: number; totalLines: number } | undefined) {
  const first = data?.startLine ?? 1;
  const last = data?.endLine ?? 0;
  return {
    first,
    last,
    earlier: Math.min(WINDOW_RADIUS, first - 1),
    later: Math.min(WINDOW_RADIUS, Math.max(0, (data?.totalLines ?? 0) - last)),
  };
}

export function coversWholeFile(
  file: { truncated?: boolean },
  moreBefore: boolean,
  moreAfter: boolean,
) {
  return !file.truncated && !moreBefore && !moreAfter;
}

type DisplayedRead = { request: WorkspaceReadFileQuery; viewer: WorkspaceFileViewer };

// Which read the reader shows: the exact previous read while its replacement is
// pending, otherwise the accepted read for this request.
function retainedFileRead({
  read,
  request,
  viewer,
  acceptedData,
  previousRead,
  previousData,
  resolving,
}: {
  read: ReturnType<typeof useWorkspaceReadFile>;
  request: WorkspaceReadFileQuery | undefined;
  viewer: WorkspaceFileViewer;
  acceptedData: ReturnType<typeof useWorkspaceReadFile>["data"];
  previousRead: DisplayedRead | undefined;
  previousData: ReturnType<typeof useCachedWorkspaceReadFile>;
  resolving: boolean;
}) {
  const retained = previousData !== undefined;
  const data = previousRead ? previousData : read.data;
  const accepted = acceptedData && request && { request, viewer };
  return {
    data,
    displayedRead: retained ? previousRead : accepted,
    retainedPending: retained && read.isFetching,
    loading: resolving || (data === undefined && (read.isPending || read.isPlaceholderData)),
  };
}

// A page or line jump keeps the exact read it replaces on screen until its own read
// is accepted, so paging never flashes an empty reader.
export function useFileReadWindow(
  workspace: ActiveSessionWorkspaceSelection,
  viewer: WorkspaceFileViewer,
  scrollPort: RefObject<HTMLDivElement | null>,
) {
  const workspaceAccess = useWorkspace();
  const cwd = resolvedWorkspaceCwd(workspace);
  const [transitionFor, setTransitionFor] = useState<{
    viewer: WorkspaceFileViewer;
    window?: AdjacentLineWindow;
    previous?: { request: WorkspaceReadFileQuery; viewer: WorkspaceFileViewer };
  }>();
  const transition = transitionFor?.viewer === viewer ? transitionFor : undefined;
  const page = transition?.window;
  const lineWindow = page ?? initialWindow(viewer.line);
  const resolving = isWorkspaceResolving(workspace);
  const request = resolving ? undefined : { cwd, path: viewer.path, ...lineWindow };
  const read = useWorkspaceReadFile(request);
  const acceptedData = read.isPlaceholderData ? undefined : read.data;
  const previousRead = acceptedData === undefined ? transition?.previous : undefined;
  // The exact origin is authoritative even if the query observer still has a placeholder.
  const previousData = useCachedWorkspaceReadFile(previousRead?.request);
  const { data, displayedRead, retainedPending, loading } = retainedFileRead({
    read,
    request,
    viewer,
    acceptedData,
    previousRead,
    previousData,
    resolving,
  });
  const lines = shownLines(data);

  // A default arrival starts at the top like its initial window; FileView places line targets.
  useLayoutEffect(() => {
    const host = scrollPort.current;
    if (host && viewer.line <= 0) host.scrollTop = 0;
  }, [scrollPort, viewer]);

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
  }, [acceptedData, page, scrollPort, transition, viewer]);

  const loadPage = (control: HTMLElement, window: AdjacentLineWindow) => {
    if (!displayedRead) return;
    if (control.ownerDocument.activeElement === control) {
      scrollPort.current?.focus({ preventScroll: true });
    }
    setTransitionFor({ viewer, window, previous: displayedRead });
  };

  const pagePending = page !== undefined && retainedPending;
  return {
    cwd,
    read,
    page,
    data,
    loading,
    shown: acceptedData !== undefined || previousData !== undefined,
    retainedPrevious: previousData !== undefined,
    awaitingPrevious: previousRead !== undefined,
    displayedRead,
    displayedViewer: displayedRead?.viewer ?? viewer,
    retainedPending,
    pagePending,
    moreBefore: lines.first > 1,
    moreAfter: lines.later > 0,
    paging: {
      earlier: lines.earlier,
      later: lines.later,
      pending: pagePending,
      unavailable: read.isPlaceholderData || retainedPending,
      fromEnd: Boolean(lineWindow.fromEnd),
      loadEarlier: (event: MouseEvent<HTMLElement>) =>
        loadPage(event.currentTarget, {
          startLine: Math.max(1, lines.first - WINDOW_RADIUS),
          endLine: lines.first - 1,
          fromEnd: true,
        }),
      loadLater: (event: MouseEvent<HTMLElement>) =>
        loadPage(event.currentTarget, {
          startLine: lines.last + 1,
          endLine: lines.last + lines.later,
        }),
    },
    goToLine: (line: number) => {
      const nextViewer = openWorkspaceFile(workspaceAccess, viewer.path, line);
      setTransitionFor(displayedRead ? { viewer: nextViewer, previous: displayedRead } : undefined);
    },
  };
}

interface FileReadFacts {
  page: AdjacentLineWindow | undefined;
  targetLine: number;
  error: unknown;
  invalidRequest: boolean;
  retained: boolean;
  retainedPrevious: boolean;
  awaitingPrevious: boolean;
  retainedPending: boolean;
  placeholderFetching: boolean;
}

// A line jump either waits, is rejected while the previous read stays, or fails.
function describeJump(t: Translate, read: FileReadFacts) {
  const jump = !read.page && read.targetLine > 0;
  const invalidLine = jump && read.invalidRequest;
  const rejectedJump = invalidLine && read.retainedPrevious;
  const failureTitle =
    !read.page && read.retainedPrevious && read.error != null
      ? t("file.error.requestedLine", { line: read.targetLine })
      : undefined;
  const pending = jump && (read.awaitingPrevious ? read.retainedPending : read.placeholderFetching);
  const pendingLabel = pending ? t("file.goToLine.pending", { line: read.targetLine }) : undefined;
  return {
    rejectedJump,
    rejectedInitialLine: invalidLine && !read.retained,
    jumpFailureTitle: failureTitle,
    jumpLabel: rejectedJump ? failureTitle : pendingLabel,
  };
}

function pageFailure(t: Translate, read: FileReadFacts) {
  if (!read.page || !read.awaitingPrevious || read.error == null) return undefined;
  return {
    title: t("file.error.requestedRange", {
      start: read.page.startLine,
      end: read.page.endLine,
    }),
    sub: "",
    size: "compact" as const,
  };
}

export function describeFileRead(t: Translate, read: FileReadFacts) {
  const jump = describeJump(t, read);
  const pageError = pageFailure(t, read);
  return {
    ...jump,
    refreshFailed:
      read.retained && read.error != null && !read.retainedPending && !jump.rejectedJump,
    pageError,
    readError: pageError ?? openingError(t, jump.rejectedInitialLine, read.targetLine),
  };
}

// The reader's own failure when no page failed: a rejected opening line or the file.
function openingError(t: Translate, rejectedLine: boolean, line: number) {
  return rejectedLine
    ? {
        title: t("file.error.requestedLine", { line }),
        sub: "",
        retryLabel: t("file.openFromStart"),
      }
    : { title: t("file.error.title"), sub: t("file.error.sub") };
}

// A whole file's length is already in its gutter; only a partial read needs its range named.
export function fileRangeSummary(
  t: ReturnType<typeof useT>,
  data: { startLine: number; endLine: number; totalLines: number },
): string {
  return t("file.range", { start: data.startLine, end: data.endLine, total: data.totalLines });
}
