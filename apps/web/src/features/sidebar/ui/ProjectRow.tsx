import { iconGlyph } from "@/ui/icons/index";
import * as stylex from "@stylexjs/stylex";
import { AgentRow } from "@/ui/agent/index";
import { Icon } from "@/ui/index";
import { StatusDot } from "@/ui/atoms/status-dot";
import { IconButton, vocab } from "@flame/ui";
import { useT } from "@/lib/i18n/index";
import { type WorkProject } from "@/features/navigation/public/workIndex";
import { color, space, type as typeStep, weight } from "@flame/ui/tokens.stylex";
const pr = stylex.create({
  count: { fontFamily: "var(--font-mono)", lineHeight: 1, color: color.fgFaint },
  line: { display: "inline-flex", minWidth: 0, alignItems: "center", gap: space.s1_5 },
  warn: { flexShrink: 0, color: color.warning },
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
  onToggle,
  onNewSession,
  canCreateSession,
}: {
  project: WorkProject;
  active: boolean;
  open: boolean;
  count: number;
  waiting: number;
  running: boolean;
  onToggle: () => void;
  onNewSession: (project: WorkProject) => void;
  canCreateSession: boolean;
}) {
  const t = useT();
  return (
    <AgentRow
      icon={open ? "folder-open" : "folder"}
      active={active}
      onClick={() => onToggle()}
      title={project.id}
      aria-expanded={open}
      trailing={
        <span {...stylex.props(pr.trailing)}>
          {!open && waiting > 0 && (
            <span {...stylex.props(pr.waiting, typeStep.uiXs)}>
              {t("project.row.waiting", { count: waiting })}
            </span>
          )}
          {!open && waiting === 0 && running && (
            <StatusDot tone="running" ring label={t("session.status.running")} />
          )}
          <span {...stylex.props(pr.count, typeStep.uiSm)}>{count}</span>
        </span>
      }
      action={
        <IconButton
          icon={iconGlyph("plus")}
          size="sm"
          data-chrome-focus=""
          aria-label={t("project.row.newSession", { name: project.name })}
          disabled={!canCreateSession}
          onClick={() => onNewSession(project)}
        />
      }
    >
      <span {...stylex.props(pr.line)}>
        <span {...stylex.props(vocab.truncate)}>{project.name}</span>
        {project.cwdMissing && (
          <Icon
            name="alert"
            size="xs"
            className={stylex.props(pr.warn).className}
            aria-label={t("project.row.missing")}
          />
        )}
      </span>
    </AgentRow>
  );
}
