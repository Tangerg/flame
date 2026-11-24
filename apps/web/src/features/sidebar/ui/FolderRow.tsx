import { iconGlyph } from "@/ui/icons/index";
import * as stylex from "@stylexjs/stylex";
import { AgentRow } from "@/ui/agent/index";
import { Icon } from "@/ui/index";
import { StatusDot } from "@/ui/atoms/status-dot";
import { IconButton, vocab } from "@flame/ui";
import { useT } from "@/lib/i18n/index";
import { type WorkFolder } from "@/features/navigation/public/workIndex";
import { color, space, type as typeStep, weight } from "@flame/ui/tokens.stylex";
const pr = stylex.create({
  count: { fontVariantNumeric: "tabular-nums", lineHeight: 1, color: color.fgFaint },
  line: { display: "inline-flex", minWidth: 0, alignItems: "center", gap: space.s1_5 },
  warn: { flexShrink: 0, color: color.warning },
  waiting: { flexShrink: 0, fontWeight: weight.medium, color: color.warning },
  trailing: { display: "inline-flex", alignItems: "center", gap: space.s1_5 },
});

export function FolderRow({
  folder,
  active,
  open,
  count,
  waiting,
  running,
  onToggle,
  onNewSession,
  canCreateSession,
}: {
  folder: WorkFolder;
  active: boolean;
  open: boolean;
  count: number;
  waiting: number;
  running: boolean;
  onToggle: () => void;
  onNewSession: (folder: WorkFolder) => void;
  canCreateSession: boolean;
}) {
  const t = useT();
  return (
    <AgentRow
      icon={open ? "folder-open" : "folder"}
      active={active}
      onClick={() => onToggle()}
      title={folder.id}
      aria-expanded={open}
      aria-label={folder.cwdMissing ? `${folder.name} — ${t("folder.row.missing")}` : undefined}
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
          aria-label={t("workIndex.row.newSession", { name: folder.name })}
          disabled={!canCreateSession}
          onClick={() => onNewSession(folder)}
        />
      }
    >
      <span {...stylex.props(pr.line)}>
        <span {...stylex.props(vocab.truncate)}>{folder.name}</span>
        {folder.cwdMissing && (
          <Icon name="alert" size="xs" className={stylex.props(pr.warn).className} />
        )}
      </span>
    </AgentRow>
  );
}
