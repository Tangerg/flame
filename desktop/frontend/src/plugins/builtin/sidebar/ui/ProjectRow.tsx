import * as stylex from "@stylexjs/stylex";
import { AgentRow } from "@/ui/agent";
import { Icon, IconButton } from "@/ui";
import { useT } from "@/lib/i18n";
import type { WorkProject } from "@/plugins/builtin/navigation/public/workIndex";
import { color, space, type as typeStep } from "@/styles/tokens.stylex";

const pr = stylex.create({
  count: { fontFamily: "var(--font-mono)", lineHeight: 1, color: color.fgFaint },
  line: { display: "inline-flex", minWidth: 0, alignItems: "center", gap: space.s1_5 },
  truncate: { overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" },
  warn: { flexShrink: 0, color: color.warning },
});

export function ProjectRow({
  project,
  active,
  open,
  count,
  onToggle,
  onNewSession,
  canCreateSession,
}: {
  project: WorkProject;
  active: boolean;
  open: boolean;
  count: number;
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
      trailing={<span {...stylex.props(pr.count, typeStep.uiSm)}>{count}</span>}
      action={
        <IconButton
          icon="plus"
          size="sm"
          iconSize="xs"
          data-chrome-focus=""
          aria-label={t("project.row.newSession", { name: project.name })}
          disabled={!canCreateSession}
          onClick={() => onNewSession(project)}
        />
      }
    >
      <span {...stylex.props(pr.line)}>
        <span {...stylex.props(pr.truncate)}>{project.name}</span>
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
