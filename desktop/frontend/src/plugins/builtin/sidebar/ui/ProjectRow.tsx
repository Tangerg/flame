import * as stylex from "@stylexjs/stylex";
import { AgentRow } from "@/ui/agent";
import { Icon, IconButton, vocab } from "@/ui";
import { useT } from "@/lib/i18n";
import type { WorkProject } from "@/plugins/builtin/navigation/public/workIndex";
import { color, corner, space, type as typeStep, weight } from "@/styles/tokens.stylex";

const pr = stylex.create({
  count: { fontFamily: "var(--font-mono)", lineHeight: 1, color: color.fgFaint },
  line: { display: "inline-flex", minWidth: 0, alignItems: "center", gap: space.s1_5 },
  warn: { flexShrink: 0, color: color.warning },
  waiting: { flexShrink: 0, fontWeight: weight.medium, color: color.warning },
  running: {
    height: space.s1_5,
    width: space.s1_5,
    flexShrink: 0,
    borderWidth: "var(--control-edge-width)",
    borderStyle: "solid",
    borderColor: color.accent,
    animation: "var(--animate-pulse-dot)",
  },
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
            <span
              role="img"
              aria-label={t("session.status.running")}
              {...stylex.props(pr.running, corner.pill)}
            />
          )}
          <span {...stylex.props(pr.count, typeStep.uiSm)}>{count}</span>
        </span>
      }
      action={
        <IconButton
          icon="plus"
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
