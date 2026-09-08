import * as stylex from "@stylexjs/stylex";
import { PROVIDERS_PANE } from "@/plugins/builtin/settings/kit/panes";
import { Icon, PillButton, Surface } from "@/ui";
import { useT } from "@/lib/i18n";
import { openWorkspaceSettingsPane } from "@/plugins/builtin/workspace/public/navigation";
import {
  needsProviderSetup,
  useProviders,
} from "@/plugins/builtin/settings/providers/public/queries";
import { color, leading, space, type as typeStep, weight } from "@/styles/tokens.stylex";

const ps = stylex.create({
  card: { width: "100%", textAlign: "left" },
  row: { display: "flex", alignItems: "flex-start", gap: space.s3 },
  glyph: { marginTop: space.s0_5, flexShrink: 0, color: color.accent },
  stack: { display: "flex", flexDirection: "column", alignItems: "flex-start", gap: space.s2 },
  title: { textWrap: "balance", color: color.fg, fontWeight: weight.semibold },
  body: { margin: 0, textWrap: "pretty", lineHeight: leading.prose, color: color.fgSoft },
  action: { marginTop: space.s0_5, fontWeight: weight.semibold },
});

export function ProviderSetupPrompt() {
  const t = useT();
  const { data: providers } = useProviders();
  if (!needsProviderSetup(providers)) return null;

  return (
    <Surface className={stylex.props(ps.card).className}>
      <div {...stylex.props(ps.row)}>
        <Icon name="spark" size="md" className={stylex.props(ps.glyph).className} />
        <div {...stylex.props(ps.stack)}>
          <div {...stylex.props(ps.title, typeStep.uiMd)}>{t("providers.setup.title")}</div>
          <p {...stylex.props(ps.body, typeStep.uiMd)}>{t("providers.setup.sub")}</p>
          <PillButton
            variant="solid"
            onClick={() => openWorkspaceSettingsPane(PROVIDERS_PANE)}
            className={stylex.props(ps.action).className}
          >
            <Icon name="settings" size="sm" />
            {t("providers.setup.action")}
          </PillButton>
        </div>
      </div>
    </Surface>
  );
}
