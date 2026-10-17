import * as stylex from "@stylexjs/stylex";
import { IconButton, vocab } from "@flame/ui";
import { space, type as typeStep, weight } from "@flame/ui/tokens.stylex";
import { useT } from "@/lib/i18n";
import type { WorkspaceViewSpec } from "@/plugins/sdk";
import { Icon, knownIconName } from "@/ui";
import { iconGlyph } from "@/ui/icons";
import { AgentSurfaceHeader } from "@/ui/agent";

const styles = stylex.create({
  title: { minWidth: 0, flex: 1, fontWeight: weight.medium },
  actions: { display: "flex", alignItems: "center", flexShrink: 0, gap: space.s0_5 },
});

export function WorkspaceWindowHeader({
  view,
  onOpenInDock,
  onClose,
}: {
  view: Pick<WorkspaceViewSpec, "title" | "icon" | "dock"> | undefined;
  onOpenInDock: () => void;
  onClose: () => void;
}) {
  const t = useT();
  const title = t(view ? view.title : "workspace.view.unavailable.title");
  const icon = knownIconName(view?.icon);
  return (
    <AgentSurfaceHeader corner="window" data-slot="workspace-window-header">
      {icon && (
        <Icon name={icon} size="md" className={stylex.props(vocab.hold, vocab.muted).className} />
      )}
      <span title={title} {...stylex.props(styles.title, vocab.truncate, typeStep.uiMd)}>
        {title}
      </span>
      <div {...stylex.props(styles.actions)}>
        {view?.dock !== undefined && (
          <IconButton
            icon={iconGlyph("open")}
            size="sm"
            aria-label={t("workspace.view.openBeside")}
            onClick={onOpenInDock}
          />
        )}
        <IconButton
          icon={iconGlyph("x")}
          size="sm"
          aria-label={t("common.close")}
          data-chrome-focus=""
          onClick={onClose}
        />
      </div>
    </AgentSurfaceHeader>
  );
}
