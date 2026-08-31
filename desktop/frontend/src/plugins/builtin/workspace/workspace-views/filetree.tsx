import { DataView } from "@/ui";
import { useT } from "@/lib/i18n";
import { useActiveSessionWorkspace } from "@/plugins/builtin/agent/public/session";
import { isUnsupportedMethod } from "@/lib/rpcErrors";
import { useWorkspaceListFiles } from "@/plugins/builtin/workspace/application/workspaceQueries";
import { FileTree } from "./views/FileTree";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import {
  openWorkspaceFile,
  useWorkspaceFileViewer,
} from "@/plugins/builtin/workspace/public/navigation";

export function ExplorerView() {
  const t = useT();
  const workspace = useActiveSessionWorkspace();
  const cwd = workspace.status === "ready" ? workspace.cwd : undefined;
  const viewer = useWorkspaceFileViewer();
  const {
    data: roots,
    isLoading,
    isError,
    error,
  } = useWorkspaceListFiles(workspace.status === "ready" ? { cwd } : undefined);

  return (
    <WorkspaceViewLayout icon="folder" titleStrong title="filetree.title">
      <DataView
        items={roots}
        isLoading={isLoading || workspace.status === "resolving"}
        isError={isError}
        error={
          isUnsupportedMethod(error)
            ? {
                icon: "folder",
                title: t("runtime.unsupported.title"),
                sub: t("runtime.unsupported.sub"),
              }
            : undefined
        }
        skeletonCount={8}
        empty={{ icon: "folder", title: t("filetree.empty.title"), sub: t("filetree.empty.sub") }}
      >
        {(rows) => (
          <FileTree
            entries={rows}
            cwd={cwd}
            selectedPath={viewer?.path}
            onSelectFile={openWorkspaceFile}
          />
        )}
      </DataView>
    </WorkspaceViewLayout>
  );
}
