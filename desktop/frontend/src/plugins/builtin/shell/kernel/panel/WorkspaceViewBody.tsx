import { Suspense } from "react";
import { EmptyState, SkeletonList } from "@/ui";
import { useT } from "@/lib/i18n";
import { PluginBoundary } from "@/plugins/host/PluginBoundary";
import { useWorkspaceViews } from "@/plugins/sdk";

interface Props {
  viewId: string;
}

export function WorkspaceViewBody({ viewId }: Props) {
  const t = useT();
  const workspaceViews = useWorkspaceViews();
  const Body = workspaceViews.find((v) => v.id === viewId)?.component;
  if (!Body) {
    return (
      <EmptyState
        icon="alert"
        title={t("workspace.view.unavailable.title")}
        sub={t("workspace.view.unavailable.body", { id: viewId })}
      />
    );
  }
  return (
    <PluginBoundary plugin={`workspace:${viewId}`} label={t("plugins.mainView")}>
      {/* The body arrives as its own chunk. Marking the wait makes it observable — a
          harness cannot otherwise tell a settled shell from one still resolving, and would
          measure a skeleton as a missing surface. */}
      <Suspense
        fallback={
          <div data-workspace-view-pending="">
            <SkeletonList count={6} label={t("common.loading")} />
          </div>
        }
      >
        <Body />
      </Suspense>
    </PluginBoundary>
  );
}
