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

export function coversWholeFile(
  file: { truncated?: boolean },
  moreBefore: boolean,
  moreAfter: boolean,
) {
  return !file.truncated && !moreBefore && !moreAfter;
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
  const data = previousRead ? previousData : read.data;
  const displayedRead =
    previousData !== undefined ? previousRead : acceptedData && request && { request, viewer };
  const retainedPending = previousData !== undefined && read.isFetching;
  const firstShown = data?.startLine ?? 1;
  const lastShown = data?.endLine ?? 0;
  const laterCount = Math.min(WINDOW_RADIUS, Math.max(0, (data?.totalLines ?? 0) - lastShown));

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
    loading: resolving || (data === undefined && (read.isPending || read.isPlaceholderData)),
    shown: acceptedData !== undefined || previousData !== undefined,
    retainedPrevious: previousData !== undefined,
    awaitingPrevious: previousRead !== undefined,
    displayedRead,
    displayedViewer: displayedRead?.viewer ?? viewer,
    retainedPending,
    pagePending,
    moreBefore: firstShown > 1,
    moreAfter: laterCount > 0,
    paging: {
      earlier: Math.min(WINDOW_RADIUS, firstShown - 1),
      later: laterCount,
      pending: pagePending,
      unavailable: read.isPlaceholderData || retainedPending,
      fromEnd: Boolean(lineWindow.fromEnd),
      loadEarlier: (event: MouseEvent<HTMLElement>) =>
        loadPage(event.currentTarget, {
          startLine: Math.max(1, firstShown - WINDOW_RADIUS),
          endLine: firstShown - 1,
          fromEnd: true,
        }),
      loadLater: (event: MouseEvent<HTMLElement>) =>
        loadPage(event.currentTarget, {
          startLine: lastShown + 1,
          endLine: lastShown + laterCount,
        }),
    },
    goToLine: (line: number) => {
      const nextViewer = openWorkspaceFile(workspaceAccess, viewer.path, line);
      setTransitionFor(displayedRead ? { viewer: nextViewer, previous: displayedRead } : undefined);
    },
  };
}

export function describeFileRead(
  t: Translate,
  read: {
    page: AdjacentLineWindow | undefined;
    targetLine: number;
    error: unknown;
    invalidRequest: boolean;
    retained: boolean;
    retainedPrevious: boolean;
    awaitingPrevious: boolean;
    retainedPending: boolean;
    placeholderFetching: boolean;
  },
) {
  const jump = !read.page && read.targetLine > 0;
  const invalidLine = jump && read.invalidRequest;
  const rejectedJump = invalidLine && read.retainedPrevious;
  const jumpFailureTitle =
    !read.page && read.retainedPrevious && read.error != null
      ? t("file.error.requestedLine", { line: read.targetLine })
      : undefined;
  const jumpPending =
    jump && (read.awaitingPrevious ? read.retainedPending : read.placeholderFetching);
  const rejectedInitialLine = invalidLine && !read.retained;
  const pageError =
    read.page && read.awaitingPrevious && read.error != null
      ? {
          title: t("file.error.requestedRange", {
            start: read.page.startLine,
            end: read.page.endLine,
          }),
          sub: "",
          size: "compact" as const,
        }
      : undefined;
  return {
    refreshFailed: read.retained && read.error != null && !read.retainedPending && !rejectedJump,
    rejectedJump,
    rejectedInitialLine,
    jumpFailureTitle,
    jumpLabel: rejectedJump
      ? jumpFailureTitle
      : jumpPending
        ? t("file.goToLine.pending", { line: read.targetLine })
        : undefined,
    pageError,
    // The reader's own failure: a failed page, a rejected opening line, or the file.
    readError:
      pageError ??
      (rejectedInitialLine
        ? {
            title: t("file.error.requestedLine", { line: read.targetLine }),
            sub: "",
            retryLabel: t("file.openFromStart"),
          }
        : { title: t("file.error.title"), sub: t("file.error.sub") }),
  };
}

export function fileLineSummary(
  t: ReturnType<typeof useT>,
  data: { startLine: number; endLine: number; totalLines: number },
  partial: boolean,
): string {
  return partial
    ? t("file.range", { start: data.startLine, end: data.endLine, total: data.totalLines })
    : t("file.lines", { count: data.totalLines });
}
