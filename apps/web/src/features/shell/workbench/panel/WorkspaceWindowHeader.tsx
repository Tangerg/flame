import * as stylex from "@stylexjs/stylex";
import { type KeyboardEventHandler, type ReactNode } from "react";
import { IconButton, vocab } from "@flame/ui";
import { space, type as typeStep, weight } from "@flame/ui/tokens.stylex";
import { useT } from "@/lib/i18n/index";
import { type WorkspaceViewSpec } from "@/features/workspace/public/views";
import { Icon, knownIconName } from "@/ui/index";
import { iconGlyph } from "@/ui/icons/index";
import { AgentSurfaceHeader } from "@/ui/agent/index";
import { ReturnButton } from "@/features/shell/workbench/panel/ReturnButton";
const styles = stylex.create({
  title: { minWidth: 0, flex: 1, fontWeight: weight.medium },
  actions: { display: "flex", alignItems: "center", flexShrink: 0, gap: space.s0_5 },
});

type WorkspaceWindowExit =
  { kind: "close"; onClose: () => void } | { kind: "return"; label: string; onReturn: () => void };

export function WorkspaceWindowHeader({
  view,
  exit,
  onOpenInDock,
  onKeyDown,
  children,
}: {
  view: Pick<WorkspaceViewSpec, "title" | "icon" | "dock"> | undefined;
  exit: WorkspaceWindowExit;
  onOpenInDock?: () => void;
  onKeyDown?: KeyboardEventHandler<HTMLDivElement>;
  children?: ReactNode;
}) {
  const t = useT();
  const title = t(view ? view.title : "workspace.view.unavailable.title");
  const icon = knownIconName(view?.icon);
  return (
    <AgentSurfaceHeader corner="window" data-slot="workspace-window-header" onKeyDown={onKeyDown}>
      {exit.kind === "return" && <ReturnButton label={exit.label} onReturn={exit.onReturn} />}
      {icon && (
        <Icon name={icon} size="md" className={stylex.props(vocab.hold, vocab.muted).className} />
      )}
      <span title={title} {...stylex.props(styles.title, vocab.truncate, typeStep.uiMd)}>
        {title}
      </span>
      <div {...stylex.props(styles.actions)}>
        {children}
        {view?.dock !== undefined && onOpenInDock && (
          <IconButton
            icon={iconGlyph("open")}
            size="sm"
            aria-label={t("workspace.view.openBeside")}
            onClick={onOpenInDock}
          />
        )}
        {exit.kind === "close" && (
          <IconButton
            icon={iconGlyph("x")}
            size="sm"
            aria-label={t("common.close")}
            data-chrome-focus=""
            onClick={exit.onClose}
          />
        )}
      </div>
    </AgentSurfaceHeader>
  );
}
