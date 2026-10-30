import { useWorkspace } from "@/features/workspace/public/workspace";
import { Button } from "@flame/ui";
import * as stylex from "@stylexjs/stylex";
import { useSettingsPaneForIntent } from "@/features/settings/public/intent";
import { Icon } from "@/ui/index";
import { Surface } from "@/ui/index";
import { useT } from "@/lib/i18n/index";
import { openWorkspaceSettingsPane } from "@/features/workspace/public/navigation";
import { needsProviderSetup } from "@/features/providers/public/queries";
import { useProviders } from "@/features/providers/public/queries";
import { color, leading, space, type as typeStep, weight } from "@flame/ui/tokens.stylex";
const ps = stylex.create({
  card: { width: "100%", textAlign: "left" },
  row: { display: "flex", alignItems: "flex-start", gap: space.s3 },
  glyph: { marginTop: space.s0_5, flexShrink: 0, color: color.accentText },
  stack: { display: "flex", flexDirection: "column", alignItems: "flex-start", gap: space.s2 },
  title: { textWrap: "balance", color: color.fg, fontWeight: weight.semibold },
  body: { margin: 0, textWrap: "pretty", lineHeight: leading.prose, color: color.fgSoft },
  action: { marginTop: space.s0_5, fontWeight: weight.semibold },
});

export function ProviderSetupPrompt() {
  const providersPane = useSettingsPaneForIntent("modelProviders");
  const workspaceAccess = useWorkspace();

  const t = useT();
  const { data: providers } = useProviders();
  if (!needsProviderSetup(providers)) return null;

  return (
    <Surface className={stylex.props(ps.card).className}>
      <div {...stylex.props(ps.row)}>
        <Icon name="server" size="md" className={stylex.props(ps.glyph).className} />
        <div {...stylex.props(ps.stack)}>
          <div {...stylex.props(ps.title, typeStep.uiMd)}>{t("providers.setup.title")}</div>
          <p {...stylex.props(ps.body, typeStep.uiMd)}>{t("providers.setup.sub")}</p>
          <Button
            variant="primary"
            onClick={() => openWorkspaceSettingsPane(workspaceAccess, providersPane)}
            className={stylex.props(ps.action).className}
          >
            <Icon name="settings" size="sm" />
            {t("providers.setup.action")}
          </Button>
        </div>
      </div>
    </Surface>
  );
}
