import { iconGlyph } from "@/ui/icons";
import * as stylex from "@stylexjs/stylex";
import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { DataView, Icon, Switch, Tag } from "@/ui";
import { Badge, EmptyState, gap, ListRow, RecordGroup, vocab } from "@flame/ui";
import { isUnsupportedMethod } from "@/lib/rpcErrors";
import type { HookReadModel } from "../application/hookConfig";
import { useHookConfigs } from "../application/hookConfig";
import { setHookTrust } from "../application/hookTrust";
import { useActiveSessionWorkspace } from "@/plugins/builtin/agent/public/session";
import { useCommandAction } from "@/plugins/sdk";
import { useT } from "@/lib/i18n";
import { color, face, leading, layout, type as typeStep, weight } from "@flame/ui/tokens.stylex";
import { SettingRow, SettingsGroup } from "../../kit";

const hp = stylex.create({
  row: {
    display: "grid",
    gridTemplateColumns: "auto minmax(0, 1fr) auto",
    alignItems: "center",
    gap: layout.contentGap,
  },
  injected: { fontStyle: "italic" },
  scope: { color: color.fgFaint, fontWeight: weight.medium },
  copy: { display: "grid", gap: layout.labelGap, minWidth: 0 },
  identity: {
    display: "flex",
    flexWrap: "wrap",
    alignItems: "center",
    gap: layout.controlGap,
    minWidth: 0,
  },
  command: {
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
        <Icon
          name={h.scope === "global" ? "globe" : "folder"}
          size="sm"
          className={stylex.props(vocab.faint).className}
        />
        <div {...stylex.props(hp.copy)}>
          <div {...stylex.props(hp.identity)}>
            <Tag>{h.event}</Tag>
            {h.matcher && (
              <span
                {...stylex.props(vocab.hold, vocab.accent, typeStep.uiSm, face.mono)}
                title={t("hooks.matcher")}
              >
                {h.matcher}
              </span>
            )}
          </div>
          <span
            {...stylex.props(hp.command, face.mono, typeStep.uiMd)}
            title={h.command || h.inject || h.source}
          >
            {h.command ? (
              h.command
            ) : (
              <span {...stylex.props(vocab.muted, hp.injected)}>{h.inject}</span>
            )}
          </span>
        </div>
        {!h.active ? (
          <Badge tone="warning" title={t("hooks.inactive.hint")}>
            {t("hooks.inactive")}
          </Badge>
        ) : h.inject ? (
          <span {...stylex.props(vocab.hold, hp.scope, typeStep.uiXs)}>
            {t("hooks.kind.inject")}
          </span>
        ) : null}
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
