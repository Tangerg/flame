import { iconGlyph } from "@/ui/icons/index";
import * as stylex from "@stylexjs/stylex";
import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { DataView } from "@/ui/index";
import { Icon } from "@/ui/index";
import { Switch } from "@flame/ui";
import { Tag } from "@/ui/index";
import { Badge, EmptyState, gap, ListRow, RecordGroup, vocab } from "@flame/ui";
import { isUnsupportedMethod } from "@/lib/rpcErrors";
import { type HookReadModel } from "@/features/hooks/application/hookConfig";
import { useHookConfigs } from "@/features/hooks/application/hookConfig";
import { setHookTrust } from "@/features/hooks/application/hookTrust";
import { useActiveSessionWorkspace } from "@/features/agent/public/session";
import { useCommandAction } from "@/features/command/public/commandAction";
import { useT } from "@/lib/i18n/index";
import { color, face, leading, layout, type as typeStep, weight } from "@flame/ui/tokens.stylex";
import { SettingRow } from "@/ui/settings/SettingRow";
import { SettingsGroup } from "@/ui/settings/SettingsGroup";
const hp = stylex.create({
  row: {
    display: "grid",
    gridTemplateColumns: "auto minmax(0, 1fr)",
    alignItems: "start",
    gap: layout.contentGap,
  },
  injected: { fontStyle: "italic" },
  scope: { color: color.fgFaint, fontWeight: weight.medium },
  copy: {
    display: "grid",
    gridTemplateColumns: "minmax(0, 1fr)",
    gap: layout.labelGap,
    minWidth: 0,
  },
  identity: {
    display: "flex",
    flexWrap: "wrap",
    alignItems: "center",
    gap: layout.controlGap,
    minWidth: 0,
  },
  code: {
    minWidth: 0,
    whiteSpace: "pre-wrap",
    overflowWrap: "anywhere",
    lineHeight: leading.body,
  },
});

function HookRow({ h }: { h: HookReadModel }) {
  const t = useT();
  return (
    <ListRow>
      <div {...stylex.props(hp.row)}>
        <span {...stylex.props(vocab.firstLine)}>
          <Icon
            name={h.scope === "global" ? "globe" : "folder"}
            size="sm"
            className={stylex.props(vocab.faint).className}
          />
        </span>
        <div {...stylex.props(hp.copy)}>
          <div {...stylex.props(hp.identity)}>
            <Tag>{h.event}</Tag>
            <span {...stylex.props(hp.scope, typeStep.uiXs)}>
              {h.scope === "global" ? t("hooks.scope.global") : t("hooks.scope.project")}
            </span>
            {!h.active ? (
              <Badge tone="warning" title={t("hooks.inactive.hint")}>
                {t("hooks.inactive")}
              </Badge>
            ) : h.inject ? (
              <span {...stylex.props(hp.scope, typeStep.uiXs)}>{t("hooks.kind.inject")}</span>
            ) : null}
          </div>
          {h.matcher && (
            <span
              {...stylex.props(hp.code, vocab.accent, typeStep.uiSm, face.mono)}
              title={t("hooks.matcher")}
            >
              {h.matcher}
            </span>
          )}
          <span {...stylex.props(hp.code, face.mono, typeStep.uiMd)} title={h.command || h.inject}>
            {h.command ? (
              h.command
            ) : (
              <span {...stylex.props(vocab.muted, hp.injected)}>{h.inject}</span>
            )}
          </span>
          <span {...stylex.props(hp.code, vocab.faint, typeStep.uiXs, face.mono)}>{h.source}</span>
        </div>
      </div>
    </ListRow>
  );
}

export function HooksPane() {
  const t = useT();
  const trust = useCommandAction({
    wasRetired: wasGenerationRetired,
    fallback: t("hooks.error.trust"),
  });
  const workspace = useActiveSessionWorkspace();
  const { data, isLoading, isError, error, refetch } = useHookConfigs(
    workspace.status === "ready" ? { cwd: workspace.cwd } : undefined,
  );

  if (isError && isUnsupportedMethod(error)) {
    return (
      <EmptyState
        icon={iconGlyph("lightning")}
        title={t("hooks.unavailable")}
        sub={t("hooks.unavailable.sub")}
      />
    );
  }

  const projectRoot = data?.projectRoot;

  return (
    <div {...stylex.props(vocab.column, gap.s4)}>
      {projectRoot && data?.hasProjectHooks && (
        <SettingsGroup>
          <SettingRow
            label={t("hooks.trust")}
            sub={
              <>
                <span>{t("hooks.trust.sub")}</span>
                <div {...stylex.props(vocab.afterLine, face.mono)}>{projectRoot}</div>
              </>
            }
          >
            <Switch
              checked={data?.projectTrusted ?? false}
              disabled={trust.busy}
              onCheckedChange={(trusted) => trust.run(() => setHookTrust(projectRoot, trusted))}
              ariaLabel={t("hooks.trust.aria")}
            />
          </SettingRow>
        </SettingsGroup>
      )}

      <DataView
        items={data?.hooks}
        isLoading={isLoading || workspace.status === "resolving"}
        failure={error}
        onRetry={refetch}
        skeletonCount={3}
        empty={{ icon: "lightning", title: t("hooks.empty"), sub: t("hooks.empty.sub") }}
      >
        {(rows) => (
          <RecordGroup variant="list">
            {rows.map((h, i) => (
              <HookRow key={`${h.source}:${h.event}:${i}`} h={h} />
            ))}
          </RecordGroup>
        )}
      </DataView>
    </div>
  );
}
