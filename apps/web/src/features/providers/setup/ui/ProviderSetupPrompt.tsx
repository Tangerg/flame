import { useWorkspace } from "@/features/workspace/public/workspace";
import { Button } from "@flame/ui";
import * as stylex from "@stylexjs/stylex";
import { useSettingsPaneForIntent } from "@/features/settings/public/intent";
import { Surface } from "@/ui/index";
import { useT } from "@/lib/i18n/index";
import { openWorkspaceSettingsPane } from "@/features/workspace/public/navigation";
import { needsProviderSetup } from "@/features/providers/public/queries";
import { useProviders } from "@/features/providers/public/queries";
import { color, space, type as typeStep } from "@flame/ui/tokens.stylex";
const ps = stylex.create({
  card: {
    display: "flex",
    flexWrap: "wrap",
    alignItems: "center",
    justifyContent: "space-between",
    gap: space.s3,
    width: "100%",
  },
  title: { minWidth: 0, color: color.fg, textWrap: "balance" },
});

export function ProviderSetupPrompt() {
  const providersPane = useSettingsPaneForIntent("model-providers");
  const workspaceAccess = useWorkspace();

  const t = useT();
  const { data: providers } = useProviders();
  if (!needsProviderSetup(providers)) return null;

  return (
    <Surface className={stylex.props(ps.card).className}>
      <div {...stylex.props(ps.title, typeStep.uiMd)}>{t("providers.setup.title")}</div>
      <Button
        variant="primary"
        onClick={() => openWorkspaceSettingsPane(workspaceAccess, providersPane)}
      >
        {t("providers.setup.action")}
      </Button>
    </Surface>
  );
}
