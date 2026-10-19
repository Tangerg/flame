import { iconGlyph } from "@/ui/icons";
import { Suspense, useCallback, useImperativeHandle, useRef, type Ref } from "react";
import * as stylex from "@stylexjs/stylex";
import { useScrollReach } from "@/ui/atoms/use-scroll-reach";
import { SkeletonList } from "@/ui";
import { EmptyState } from "@flame/ui";
import { useT } from "@/lib/i18n";
import { PluginBoundary } from "@/plugins/host/PluginBoundary";
import { useWorkspaceViews } from "@/plugins/sdk";

interface Props {
  viewId: string;
  ref?: Ref<HTMLDivElement>;
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

export function WorkspaceViewBody({ viewId, ref }: Props) {
  const t = useT();
  const scroll = useScrollReach();
  const element = useRef<HTMLDivElement>(null);
  const scrollRef = scroll.ref;
  const attach = useCallback(
    (node: HTMLDivElement | null) => {
      element.current = node;
      return scrollRef(node);
    },
    [scrollRef],
  );
  useImperativeHandle(ref, () => element.current!, []);
  const workspaceViews = useWorkspaceViews();
  const view = workspaceViews.find((v) => v.id === viewId);
  const Body = view?.component;
  return (
    <div
      role="region"
      aria-label={t(view?.title ?? "plugins.mainView")}
      {...scroll}
      ref={attach}
      tabIndex={scroll.tabIndex ?? -1}
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
          {Body ? (
            <Body />
          ) : (
            <EmptyState
              icon={iconGlyph("alert")}
              title={t("workspace.view.unavailable.title")}
              sub={t("workspace.view.unavailable.body", { id: viewId })}
            />
          )}
        </Suspense>
      </PluginBoundary>
    </div>
  );
}
