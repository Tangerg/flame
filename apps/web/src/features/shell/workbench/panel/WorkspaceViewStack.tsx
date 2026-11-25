import * as stylex from "@stylexjs/stylex";
import { Activity } from "react";
import { PanelHost } from "@flame/ui/panel";
import { ContentVisibility } from "@/ui/index";
import { useContextDockCatalog } from "@/features/workspace/public/contextDockCatalog";
import { dockViewMarker, WORKSPACE_DOCK_CATALOG } from "@/features/workspace/public/navigation";
import { WorkspaceViewBody } from "@/features/shell/workbench/panel/WorkspaceViewBody";
import { useT } from "@/lib/i18n/index";
import { shellStyles as sh } from "@/features/shell/workbench/shellStyles";
import {
  CATALOG_VIEW,
  type InspectionFocus,
} from "@/features/shell/workbench/panel/workspacePresentation";
import { DockCatalogPage } from "@/features/shell/workbench/panel/DockHeader";
import { paneStyles } from "@/features/shell/workbench/panel/ConversationPane";

export function WorkspaceViewStack({
  presentation,
  shown,
  presentedViewId,
  fullPaneViewId,
  addressable,
  mountedViewIds,
  mainViewRef,
  presentationRef,
  catalog,
  openViewIds,
}: {
  presentation: "main" | "dock";
  shown: boolean;
  presentedViewId: string | null;
  fullPaneViewId: string | null;
  addressable: boolean;
  mountedViewIds: ReadonlySet<string>;
  mainViewRef: InspectionFocus["mainViewRef"];
  presentationRef: InspectionFocus["presentationRef"];
  catalog: ReturnType<typeof useContextDockCatalog>;
  openViewIds: ReadonlySet<string>;
}) {
  const t = useT();
  const main = presentation === "main";
  return (
    <div {...stylex.props(sh.anchor)}>
      <Activity mode={presentedViewId === WORKSPACE_DOCK_CATALOG ? "visible" : "hidden"}>
        <div
          ref={fullPaneViewId === WORKSPACE_DOCK_CATALOG ? mainViewRef : undefined}
          role={main ? "region" : undefined}
          aria-label={main ? t(CATALOG_VIEW.title) : undefined}
          tabIndex={main ? -1 : undefined}
          data-focus-inset=""
          {...dockViewMarker(WORKSPACE_DOCK_CATALOG)}
          {...stylex.props(sh.overlay)}
        >
          <DockCatalogPage groups={catalog} openViewIds={openViewIds} titleHidden={main} />
        </div>
      </Activity>
      {/* Moving retained readers with the tab strip can reset their WebKit scroll positions. */}
      {[...mountedViewIds].sort().map((viewId) => {
        const visible = viewId === presentedViewId && shown;
        return (
          <div
            key={viewId}
            inert={!visible}
            aria-hidden={!visible || undefined}
            {...dockViewMarker(addressable ? viewId : undefined)}
            {...stylex.props(sh.overlay, !visible && paneStyles.inactiveView)}
          >
            <ContentVisibility visible={visible} fallbackFocus={presentationRef}>
              <PanelHost density={main ? "page" : "compact"}>
                <WorkspaceViewBody
                  ref={viewId === fullPaneViewId ? mainViewRef : undefined}
                  viewId={viewId}
                  presentation={presentation}
                />
              </PanelHost>
            </ContentVisibility>
          </div>
        );
      })}
    </div>
  );
}
