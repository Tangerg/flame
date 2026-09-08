import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import type { IconName } from "@/ui";
import { AgentSurfaceHeader } from "@/ui/agent";
import { Icon, IconButton, vocab } from "@/ui";
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
  /**
   * What the title IS. Prose is a view's name; `mono` is machine text — a path, a command.
   *
   * It was `?: boolean`, defaulting to mono: a name that said WEIGHT and switched
   * FACE, with nineteen of twenty-one call sites passing the flag to opt out of the default.
   * A default that all but two callers override is not a default.
   */
  titleFace?: "prose" | "mono";
}

export function ViewHeader({
  icon,
  title,
  dockIdentity,
  sub,
  actions,
  titleFace,
}: ViewHeaderProps) {
  const placement = useViewPlacement();
  if (placement?.placement === "dock") {
    return <DockViewBar identity={dockIdentity} sub={sub} actions={actions} />;
  }
  return (
    <FullViewBar icon={icon} title={title} sub={sub} actions={actions} titleFace={titleFace} />
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
      <div {...stylex.props(vocab.line, vocab.fill, vocab.muted, typeStep.uiMd, face.mono)}>
        {identity !== undefined && <span {...stylex.props(vocab.fill)}>{identity}</span>}
        {identity !== undefined && sub !== undefined && (
          <span aria-hidden {...stylex.props(vocab.hold, vs.dotSep)}>
            ·
          </span>
        )}
        {sub !== undefined && (
          <span {...stylex.props(vocab.truncate, identity === undefined ? vocab.fill : vocab.hold)}>
            {sub}
          </span>
        )}
      </div>
      {actions !== undefined && <div {...stylex.props(vs.actionsTight)}>{actions}</div>}
    </AgentSurfaceHeader>
  );
}

function FullViewBar({ icon, title, sub, actions, titleFace = "prose" }: ViewHeaderProps) {
  const placement = useViewPlacement();
  const t = useT();

  return (
    <AgentSurfaceHeader corner="window">
      <Icon name={icon} size="md" className={stylex.props(vocab.hold, vocab.muted).className} />
      <div {...stylex.props(vocab.line, vocab.fill)}>
        <span
          {...stylex.props(
            vocab.min,
            vocab.truncate,
            vs.titleMedium,
            typeStep.uiMd,
            titleFace === "mono" && face.mono,
          )}
        >
          {typeof title === "string" ? t(title) : title}
        </span>
        {sub !== undefined && (
          <>
            <span aria-hidden="true" {...stylex.props(vocab.hold, vs.dotSep, typeStep.uiMd)}>
              ·
            </span>
            <span
              {...stylex.props(vocab.min, vocab.truncate, vocab.muted, typeStep.uiMd, face.mono)}
            >
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
