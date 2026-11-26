import { useNotifications } from "@/features/notifications/public/notifier";
import { useWorkspace } from "@/features/workspace/public/workspace";
import * as stylex from "@stylexjs/stylex";
import { useLayoutEffect, useRef, type Ref } from "react";
import { SkeletonList } from "@/ui/index";
import { ContentVisibility } from "@flame/ui";
import { openLocalFile, revealLocalFile } from "@/features/workspace/application/localFileActions";
import { useT } from "@/lib/i18n/index";
import { WorkspaceViewLayout } from "@/features/workspace/ui/WorkspaceViewLayout";
import {
  isWorkspaceResolving,
  resolvedWorkspaceCwd,
  useActiveSessionWorkspace,
  type ActiveSessionWorkspaceSelection,
} from "@/features/agent/public/session";
import { openWorkspaceFile, useWorkspaceFileViewer } from "@/features/workspace/public/navigation";
import { type WorkspaceFileViewer } from "@/features/workspace/application/navigationState";
import { FileTree } from "@/features/workspace/ui/files/FileTree";
import { useAsyncFeedback } from "@/lib/useAsyncFeedback";
import {
  type LocalFileActions,
  LocalFileFeedback,
} from "@/features/workspace/ui/files/LocalFileFeedback";
import { FilePreview } from "@/features/workspace/ui/files/FilePreview";

const styles = stylex.create({
  workspace: {
    position: "relative",
    display: "flex",
    flex: 1,
    flexDirection: "column",
    minWidth: 0,
    minHeight: 0,
  },
  // A zero-sized viewport invalidates the virtual tree's measured row layout.
  browser: { position: "absolute", inset: 0, display: "flex", flexDirection: "column" },
  hidden: { visibility: "hidden" },
});

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
  const browserVisible = !viewer;
  return (
    <div ref={panelRef} {...stylex.props(styles.workspace)}>
      <ContentVisibility visible={browserVisible}>
        <div
          inert={!browserVisible}
          aria-hidden={!browserVisible || undefined}
          {...stylex.props(styles.browser, !browserVisible && styles.hidden)}
        >
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
      beforeScroll={<LocalFileFeedback actions={fileActions} focusTarget={treeRef} />}
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
