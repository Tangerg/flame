import { iconGlyph } from "@/ui/icons";
import { Suspense } from "react";
import * as stylex from "@stylexjs/stylex";
import { useScrollReach } from "@/ui/atoms/use-scroll-reach";
import { SkeletonList } from "@/ui";
import { EmptyState } from "@flame/ui";
import { useT } from "@/lib/i18n";
import { PluginBoundary } from "@/plugins/host/PluginBoundary";
import { useWorkspaceViews } from "@/plugins/sdk";

interface Props {
  viewId: string;
}

const styles = stylex.create({
  viewport: {
    display: "flex",
    flex: 1,
    flexDirection: "column",
    minWidth: 0,
    minHeight: 0,
    overflowY: "auto",
    overscrollBehavior: "contain",
    userSelect: "text",
  },
});

export function WorkspaceViewBody({ viewId }: Props) {
  const t = useT();
  const scroll = useScrollReach();
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
    <div
      {...scroll}
      data-focus-inset=""
      data-slot="workspace-view-body"
      {...stylex.props(styles.viewport)}
    >
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
    </div>
  );
}
