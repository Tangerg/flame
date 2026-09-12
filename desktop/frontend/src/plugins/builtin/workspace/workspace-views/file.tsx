import { Activity } from "react";
import { DataView, FilePath, IconButton } from "@/ui";
import { useT } from "@/lib/i18n";
import { FileView } from "./views/FileView";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { useActiveSessionWorkspace } from "@/plugins/builtin/agent/public/session";
import {
  useWorkspaceListFiles,
  useWorkspaceReadFile,
} from "@/plugins/builtin/workspace/application/workspaceQueries";
import {
  closeWorkspaceFile,
  openWorkspaceFile,
  useWorkspaceFileViewer,
} from "@/plugins/builtin/workspace/public/navigation";
import type { WorkspaceFileViewer } from "../application/ports/navigationState";
import { isUnsupportedMethod } from "@/lib/rpcErrors";
import { FileTree } from "./views/FileTree";

const targetWindowRadius = 200;

export function FileViewTab() {
  const viewer = useWorkspaceFileViewer();
  return (
    <>
      <Activity mode={viewer ? "hidden" : "visible"}>
        <FileBrowser />
      </Activity>
      {viewer && <FilePreview viewer={viewer} />}
    </>
  );
}

function FileBrowser() {
  const t = useT();
  const workspace = useActiveSessionWorkspace();
  const cwd = workspace.status === "ready" ? workspace.cwd : undefined;
  const query = useWorkspaceListFiles(workspace.status === "ready" ? { cwd } : undefined);
  return (
    <WorkspaceViewLayout scrollInset="flush" icon="folder" title="workspace.view.title.file">
      <DataView
        items={query.data}
        isLoading={query.isLoading || workspace.status === "resolving"}
        isError={query.isError}
        onRetry={query.refetch}
        unsupported={
          isUnsupportedMethod(query.error)
            ? {
                icon: "folder",
                title: t("runtime.unsupported.title"),
                sub: t("runtime.unsupported.sub"),
              }
            : undefined
        }
        skeletonCount={8}
        empty={{ icon: "folder", title: t("file.empty.title"), sub: t("file.empty.sub") }}
      >
        {(entries) => <FileTree entries={entries} cwd={cwd} onSelectFile={openWorkspaceFile} />}
      </DataView>
    </WorkspaceViewLayout>
  );
}

function FilePreview({ viewer }: { viewer: WorkspaceFileViewer }) {
  const t = useT();
  const workspace = useActiveSessionWorkspace();
  const cwd = workspace.status === "ready" ? workspace.cwd : undefined;
  const targetLine = viewer.line;
  const { data, isLoading, isError, refetch } = useWorkspaceReadFile(
    workspace.status === "ready"
      ? {
          cwd,
          path: viewer.path,
          ...(targetLine > 0
            ? {
                startLine: Math.max(1, targetLine - targetWindowRadius),
                endLine: targetLine + targetWindowRadius,
              }
            : {}),
        }
      : undefined,
  );

  const sub = data ? (
    <span>
      {t("file.lines", { count: data.totalLines })}
      {data.truncated && ` · ${t("file.truncated")}`}
    </span>
  ) : undefined;

  return (
    <WorkspaceViewLayout
      scrollInset="flush"
      titleFace="mono"
      icon="filetext"
      title={viewer.path}
      dockIdentity={<FilePath path={viewer.path} />}
      actions={
        <IconButton icon="arrow-left" title={t("file.backToFiles")} onClick={closeWorkspaceFile} />
      }
      sub={sub}
    >
      <DataView
        items={data ? [data] : []}
        isLoading={isLoading || workspace.status === "resolving"}
        isError={isError}
        onRetry={refetch}
        skeletonCount={12}
        error={{ title: t("file.error.title"), sub: t("file.error.sub") }}
      >
        {(items) => (
          <FileView
            path={viewer.path}
            content={items[0]!.content}
            startLine={items[0]!.startLine}
            targetLine={targetLine}
          />
        )}
      </DataView>
    </WorkspaceViewLayout>
  );
}
