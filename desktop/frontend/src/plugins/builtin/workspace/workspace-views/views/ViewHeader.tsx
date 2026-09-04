import type { ReactNode } from "react";
import type { IconName } from "@/ui";
import { AgentSurfaceHeader } from "@/ui/agent";
import { Icon, IconButton } from "@/ui";
import { cn } from "@/lib/classNames";
import { useT } from "@/lib/i18n";
import { useViewPlacement } from "@/plugins/builtin/workspace/public/viewPlacement";

export interface ViewHeaderProps {
  icon: IconName;
  title: ReactNode;
  dockIdentity?: ReactNode;
  sub?: ReactNode;
  actions?: ReactNode;
  titleStrong?: boolean;
}

export function ViewHeader({
  icon,
  title,
  dockIdentity,
  sub,
  actions,
  titleStrong,
}: ViewHeaderProps) {
  const placement = useViewPlacement();
  if (placement?.placement === "dock") {
    return <DockViewBar identity={dockIdentity} sub={sub} actions={actions} />;
  }
  return (
    <FullViewBar icon={icon} title={title} sub={sub} actions={actions} titleStrong={titleStrong} />
  );
}

function DockViewBar({
  identity,
  sub,
  actions,
}: Pick<ViewHeaderProps, "sub" | "actions"> & { identity?: ReactNode }) {
  if (identity === undefined && sub === undefined && actions === undefined) return null;
  return (
    <AgentSurfaceHeader className="gap-2">
      <div className="flex min-w-0 flex-1 items-center gap-2 font-mono text-ui-md text-fg-muted">
        {identity !== undefined && <span className="min-w-0 flex-1">{identity}</span>}
        {identity !== undefined && sub !== undefined && (
          <span aria-hidden className="shrink-0 leading-none text-fg-faint">
            ·
          </span>
        )}
        {sub !== undefined && (
          <span className={cn("truncate", identity === undefined ? "min-w-0 flex-1" : "shrink-0")}>
            {sub}
          </span>
        )}
      </div>
      {actions !== undefined && <div className="flex shrink-0 items-center gap-1">{actions}</div>}
    </AgentSurfaceHeader>
  );
}

function FullViewBar({ icon, title, sub, actions, titleStrong }: ViewHeaderProps) {
  const placement = useViewPlacement();
  const t = useT();

  return (
    <AgentSurfaceHeader className="gap-2" corner="window">
      <Icon name={icon} size="md" className="shrink-0 text-fg-muted" />
      <div className="flex min-w-0 flex-1 items-center gap-2">
        <span
          className={cn(
            "min-w-0 truncate text-ui-md font-medium text-fg",
            titleStrong ? "font-sans" : "font-mono",
          )}
        >
          {typeof title === "string" ? t(title) : title}
        </span>
        {sub !== undefined && (
          <>
            <span aria-hidden="true" className="shrink-0 text-ui-md leading-none text-fg-faint">
              ·
            </span>
            <span className="min-w-0 truncate font-mono text-ui-md text-fg-muted">{sub}</span>
          </>
        )}
      </div>
      <div className="flex shrink-0 items-center gap-1">
        {actions}
        {placement?.splittable && (
          <IconButton
            icon="panel-r"
            size="sm"
            title={t("workspace.view.openBeside")}
            onClick={placement.onOpenInDock}
          />
        )}
        {placement && (
          <IconButton icon="x" size="sm" title={t("common.close")} onClick={placement.onClose} />
        )}
      </div>
    </AgentSurfaceHeader>
  );
}
