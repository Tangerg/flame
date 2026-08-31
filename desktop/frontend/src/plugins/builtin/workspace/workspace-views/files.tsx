import { DataView } from "@/ui";
import { useT } from "@/lib/i18n";
import { FilesChanged } from "./views/FilesChanged";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { useActiveSessionWorkspace } from "@/plugins/builtin/agent/public/session";
import { useWorkspaceFileChanges } from "@/plugins/builtin/workspace/application/workspaceQueries";
import {
  fileChangesSubtext,
  fileChangesViewModel,
} from "@/plugins/builtin/workspace/application/fileChangesViewModel";
import {
  openWorkspaceDiffForFile,
  useWorkspaceFileFocus,
} from "@/plugins/builtin/workspace/public/navigation";
import { gitOffEmpty, isVcsUnavailable, notARepoEmpty } from "./views/vcsGate";
import { useWorkspaceCapability } from "@/plugins/builtin/workspace/application/workspaceCapabilities";

export function FilesView() {
  const t = useT();
  const gitEnabled = useWorkspaceCapability("git");
  const workspace = useActiveSessionWorkspace();
  const fileFocus = useWorkspaceFileFocus();
  const {
    data: files,
    isLoading,
    isError,
    error,
  } = useWorkspaceFileChanges(
    gitEnabled && workspace.status === "ready" ? { cwd: workspace.cwd } : undefined,
  );
  const items = files ?? [];
  const view = fileChangesViewModel(items, fileFocus.path);
  const notARepo = isVcsUnavailable(error);

  return (
    <WorkspaceViewLayout
      icon="filetext"
      titleStrong
      title="files.title"
      sub={fileChangesSubtext(t, view)}
    >
      <DataView
        items={gitEnabled ? items : []}
        isLoading={isLoading || workspace.status === "resolving"}
        isError={isError && !notARepo}
        skeletonCount={6}
        empty={
          !gitEnabled
            ? gitOffEmpty("filetext")
            : notARepo
              ? notARepoEmpty("filetext")
              : {
                  icon: "check",
                  title: t("files.empty.title"),
                  sub: t("files.empty.sub"),
                }
        }
      >
        {(rows) => (
          <FilesChanged
            view={fileChangesViewModel(rows, fileFocus.path)}
            onSelect={openWorkspaceDiffForFile}
          />
        )}
      </DataView>
    </WorkspaceViewLayout>
  );
}
