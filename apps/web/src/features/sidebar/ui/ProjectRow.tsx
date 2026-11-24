import { iconGlyph } from "@/ui/icons/index";
import * as stylex from "@stylexjs/stylex";
import { AgentRow } from "@/ui/agent/index";
import { StatusDot } from "@/ui/atoms/status-dot";
import { IconButton, vocab } from "@flame/ui";
import { useT } from "@/lib/i18n/index";
import { type WorkProject } from "@/features/navigation/public/workIndex";
import { color, space, type as typeStep, weight } from "@flame/ui/tokens.stylex";
const pr = stylex.create({
  count: { fontVariantNumeric: "tabular-nums", lineHeight: 1, color: color.fgFaint },
  waiting: { flexShrink: 0, fontWeight: weight.medium, color: color.warning },
  trailing: { display: "inline-flex", alignItems: "center", gap: space.s1_5 },
});

export function ProjectRow({
  project,
  active,
  open,
  count,
  waiting,
  running,
  onOpen,
  onNewSession,
  canCreateSession,
}: {
  project: WorkProject;
  active: boolean;
  open: boolean;
  count: number;
  waiting: number;
  running: boolean;
  onOpen: () => void;
  onNewSession: () => void;
  canCreateSession: boolean;
}) {
  const t = useT();
  return (
    <AgentRow
      icon="blocks"
      active={active}
      onClick={onOpen}
      trailing={
        <span {...stylex.props(pr.trailing)}>
          {!open && waiting > 0 && (
            <span {...stylex.props(pr.waiting, typeStep.uiXs)}>
              {t("workIndex.row.waiting", { count: waiting })}
            </span>
          )}
          {!open && waiting === 0 && running && (
            <StatusDot tone="running" ring label={t("session.status.running")} />
          )}
          {!open && <span {...stylex.props(pr.count, typeStep.uiSm)}>{count}</span>}
        </span>
      }
      action={
        <IconButton
          icon={iconGlyph("plus")}
          size="sm"
          data-chrome-focus=""
          aria-label={t("workIndex.row.newSession", { name: project.name })}
          disabled={!canCreateSession}
          onClick={onNewSession}
        />
      }
    >
      <span {...stylex.props(vocab.truncate)}>{project.name}</span>
    </AgentRow>
  );
}
