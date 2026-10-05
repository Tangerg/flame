import {
  forgetRules,
  allowMCPTool,
  useApprovalRules,
} from "@/plugins/builtin/agent/public/approvalPolicy";
import { setMCPToolExposure } from "../application/mcpServerConfig";
import { useCommandAction } from "@/plugins/sdk";
import { wasGenerationRetired } from "@/lib/asyncOwnership";
import * as stylex from "@stylexjs/stylex";
import { Button, DataView, EmptyState, Switch, vocab } from "@/ui";
import { useT } from "@/lib/i18n";
import {
  sameMCPServer,
  useMCPTools,
  useMCPToolExposure,
  type MCPServerID,
} from "../application/mcpServerQueries";
import {
  color,
  motion,
  radius,
  space,
  surface,
  type as typeStep,
  weight,
} from "@/styles/tokens.stylex";
import { settingStyles as ss } from "../../kit/settingStyles";

interface Props {
  server: MCPServerID;
}

const tc = stylex.create({
  panel: { borderRadius: radius.card, backgroundColor: surface.sunken, padding: space.s2_5 },
  grid: {
    display: "grid",
    gridTemplateColumns: "minmax(0, 1fr) auto auto",
    alignItems: "center",
    columnGap: space.s4,
  },
  head: {
    rowGap: space.s1,
    paddingInline: space.s1_5,
    paddingBottom: space.s1_5,
    color: color.fgMuted,
    fontWeight: weight.medium,
  },
  row: {
    borderRadius: radius.card,
    backgroundColor: { default: null, ":hover": surface.hover },
    paddingInline: space.s1_5,
    paddingBlock: space.s1_5,
    transitionProperty: "background-color",
    transitionTimingFunction: motion.easeState,
  },
  switchCol: { width: space.s12, textAlign: "center" },
  switchCell: { display: "flex", width: space.s12, justifyContent: "center" },
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
      <div {...stylex.props(tc.grid, tc.head, typeStep.uiSm)}>
        <span>{t("mcp.tools.tool")}</span>
        <span {...stylex.props(tc.switchCol)}>{t("mcp.tools.enabled")}</span>
        <span {...stylex.props(tc.switchCol)}>{t("mcp.tools.autoApprove")}</span>
      </div>
      {exposure.error != null && (
        <EmptyState
          icon="alert"
          title={t("mcp.tools.enabled")}
          sub={t("dataView.error.sub")}
          action={<Button onClick={() => exposure.refetch()}>{t("common.retry")}</Button>}
        />
      )}
      {rules.error != null && (
        <EmptyState
          icon="alert"
          title={t("mcp.tools.autoApprove")}
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
          <div {...stylex.props(vocab.column)}>
            {tools.map((tool) => {
              const isDisabled = disabled.has(tool.name);
              return (
                <div key={tool.name} {...stylex.props(tc.grid, tc.row)}>
                  <div>
                    <code
                      {...stylex.props(ss.monoName, typeStep.uiMd)}
                      title={tool.description || tool.name}
                    >
                      {tool.name}
                    </code>
                    {tool.nameConflicts.length > 0 && (
                      <p {...stylex.props(typeStep.uiSm, vocab.muted)}>
                        {t("mcp.tools.nameConflict", {
                          name: tool.modelName,
                          sources: tool.nameConflicts.join(", "),
                        })}
                      </p>
                    )}
                  </div>
                  <div {...stylex.props(tc.switchCell)}>
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
                  </div>
                  <div {...stylex.props(tc.switchCell)}>
                    <Switch
                      checked={allowRules(tool.name).some((rule) => !rule.stale)}
                      disabled={
                        approvalAction.busy || rules.isLoading || rules.error != null || !rules.data
                      }
                      onCheckedChange={(on) => setAutoApprove(tool.name, on)}
                      ariaLabel={t("mcp.tools.autoApprove.aria", { tool: tool.name })}
                    />
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </DataView>
    </div>
  );
}
