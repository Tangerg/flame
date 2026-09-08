import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import type { IconName } from "@/ui";
import { AgentSurfaceHeader } from "@/ui/agent";
import { Icon, IconButton } from "@/ui";
import { cn } from "@/lib/classNames";
import { useT } from "@/lib/i18n";
import { useViewPlacement } from "@/plugins/builtin/workspace/public/viewPlacement";
import { face, type as typeStep } from "@/styles/tokens.stylex";
import { viewStyles as vs } from "./viewStyles";

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
    <AgentSurfaceHeader>
      <div {...stylex.props(vs.line, vs.fill, vs.muted, typeStep.uiMd, face.mono)}>
        {identity !== undefined && <span {...stylex.props(vs.fill)}>{identity}</span>}
        {identity !== undefined && sub !== undefined && (
          <span aria-hidden {...stylex.props(vs.hold, vs.dotSep)}>
            ·
          </span>
        )}
        {sub !== undefined && (
          <span className={cn("truncate", identity === undefined ? "min-w-0 flex-1" : "shrink-0")}>
            {sub}
          </span>
        )}
      </div>
      {actions !== undefined && <div {...stylex.props(vs.actionsTight)}>{actions}</div>}
    </AgentSurfaceHeader>
  );
}

function FullViewBar({ icon, title, sub, actions, titleStrong }: ViewHeaderProps) {
  const placement = useViewPlacement();
  const t = useT();

  return (
    <AgentSurfaceHeader corner="window">
      <Icon name={icon} size="md" className={stylex.props(vs.hold, vs.muted).className} />
      <div {...stylex.props(vs.line, vs.fill)}>
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
            <span aria-hidden="true" {...stylex.props(vs.hold, vs.dotSep, typeStep.uiMd)}>
              ·
            </span>
            <span {...stylex.props(vs.min, vs.truncate, vs.muted, typeStep.uiMd, face.mono)}>
              {sub}
            </span>
          </>
        )}
      </div>
      <div {...stylex.props(vs.actionsTight)}>
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
