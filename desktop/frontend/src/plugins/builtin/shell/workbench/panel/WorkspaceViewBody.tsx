import { iconGlyph } from "@/ui/icons";
import { Suspense } from "react";
import { SkeletonList } from "@/ui";
import { EmptyState } from "@flame/ui";
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
        icon={iconGlyph("alert")}
        title={t("workspace.view.unavailable.title")}
        sub={t("workspace.view.unavailable.body", { id: viewId })}
      />
    );
  }
  return (
    <PluginBoundary plugin={`workspace:${viewId}`} label={t("plugins.mainView")}>
      <Suspense
        fallback={
          <div data-surface-pending="">
            <SkeletonList count={6} label={t("common.loading")} />
          </div>
        }
      >
        <Body />
      </Suspense>
    </PluginBoundary>
  );
}
