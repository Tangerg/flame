import { iconGlyph } from "@/ui/icons/index";
import { forgetRules, allowMCPTool } from "@/features/agent/public/approvalPolicy";
import { useApprovalRules } from "@/features/agent/public/approvalPolicy";
import { setMCPToolExposure } from "@/features/mcp/application/mcpServerConfig";
import { useCommandAction } from "@/features/command/public/commandAction";
import { wasGenerationRetired } from "@/lib/asyncOwnership";
import * as stylex from "@stylexjs/stylex";
import { DataView } from "@/ui/index";
import { Switch } from "@flame/ui";
import { Button, EmptyState, vocab } from "@flame/ui";
import { useT } from "@/lib/i18n/index";
import {
  sameMCPServer,
  useMCPTools,
  useMCPToolExposure,
  type MCPServerID,
} from "@/features/mcp/application/mcpServerQueries";
import { layout, type as typeStep } from "@flame/ui/tokens.stylex";
import { SettingRow } from "@/ui/settings/SettingRow";
import { SettingsGroup } from "@/ui/settings/SettingsGroup";
interface Props {
  server: MCPServerID;
}

const tc = stylex.create({
  panel: { display: "grid", gap: layout.contentGap, minWidth: 0 },
  controls: {
    display: "flex",
    flexWrap: "wrap",
    alignItems: "center",
    justifyContent: "flex-end",
    gap: layout.contentGap,
    minWidth: 0,
  },
  choice: {
    display: "inline-flex",
    alignItems: "center",
    gap: layout.controlGap,
    minWidth: 0,
    maxWidth: "100%",
  },
});

export function ToolControls({ server }: Props) {
  const t = useT();
  const tools = useMCPTools({ server });
  const exposure = useMCPToolExposure({ server });
  const rules = useApprovalRules({});
  const approvalAction = useCommandAction({
    wasRetired: wasGenerationRetired,
    fallback: t("mcp.error.toolPolicy"),
  });
  const exposureAction = useCommandAction({
    wasRetired: wasGenerationRetired,
    fallback: t("mcp.error.toolPolicy"),
  });
  const disabled = new Set(exposure.data);
  const sourceRules =
    rules.data?.filter(
      (rule) =>
        rule.tool.type === "mcp" &&
        sameMCPServer(rule.tool.server, server) &&
        rule.scope === "global" &&
        rule.subject.type === "all",
    ) ?? [];
  const allowRules = (name: string) =>
    sourceRules.filter(
      (rule) => rule.tool.type === "mcp" && rule.tool.name === name && rule.decision === "allow",
    );
  const setAutoApprove = (name: string, on: boolean) =>
    approvalAction.run(() =>
      on ? allowMCPTool(server, name) : forgetRules(allowRules(name).map((rule) => rule.id)),
    );
  return (
    <div {...stylex.props(tc.panel)}>
      <p {...stylex.props(typeStep.uiSm, vocab.muted)}>{t("mcp.tools.policyHint")}</p>
      {exposure.error != null && (
        <EmptyState
          icon={iconGlyph("alert")}
          tone="negative"
          title={t("dataView.error.title")}
          sub={t("dataView.error.sub")}
          action={<Button onClick={() => exposure.refetch()}>{t("common.retry")}</Button>}
        />
      )}
      {rules.error != null && (
        <EmptyState
          icon={iconGlyph("alert")}
          tone="negative"
          title={t("dataView.error.title")}
          sub={t("dataView.error.sub")}
          action={<Button onClick={() => rules.refetch()}>{t("common.retry")}</Button>}
        />
      )}
      <DataView
        items={tools.data}
        isLoading={tools.isLoading}
        failure={tools.error}
        onRetry={tools.refetch}
        skeletonCount={3}
        empty={{ icon: "tool", title: t("mcp.tools.empty") }}
      >
        {(tools) => (
          <SettingsGroup>
            {tools.map((tool) => {
              const isDisabled = disabled.has(tool.name);
              return (
                <SettingRow
                  key={tool.name}
                  label={tool.name}
                  sub={
                    <>
                      {tool.description}
                      {tool.nameConflicts.length > 0 && (
                        <p {...stylex.props(typeStep.uiSm, vocab.muted)}>
                          {t("mcp.tools.nameConflict", {
                            name: tool.modelName,
                            sources: tool.nameConflicts.join(", "),
                          })}
                        </p>
                      )}
                    </>
                  }
                >
                  <div {...stylex.props(tc.controls)}>
                    <label {...stylex.props(tc.choice, typeStep.uiSm)}>
                      <span>{t("mcp.tools.enabled")}</span>
                      <Switch
                        checked={!isDisabled}
                        disabled={
                          exposureAction.busy ||
                          exposure.isLoading ||
                          exposure.error != null ||
                          !exposure.data
                        }
                        onCheckedChange={(on) =>
                          exposureAction.run(() => setMCPToolExposure(server, tool.name, !on))
                        }
                        ariaLabel={t("mcp.tools.enable.aria", { tool: tool.name })}
                      />
                    </label>
                    <label {...stylex.props(tc.choice, typeStep.uiSm)}>
                      <span>{t("mcp.tools.autoApprove")}</span>
                      <Switch
                        checked={allowRules(tool.name).some((rule) => !rule.stale)}
                        disabled={
                          approvalAction.busy ||
                          rules.isLoading ||
                          rules.error != null ||
                          !rules.data
                        }
                        onCheckedChange={(on) => setAutoApprove(tool.name, on)}
                        ariaLabel={t("mcp.tools.autoApprove.aria", { tool: tool.name })}
                      />
                    </label>
                  </div>
                </SettingRow>
              );
            })}
          </SettingsGroup>
        )}
      </DataView>
    </div>
  );
}
