import { useEffect, useEffectEvent, useSyncExternalStore } from "react";
import type { ComponentContext, Translate } from "@flame/component-sdk";
import type { MCPServerID, ToolRef } from "@flame/runtime-client/wire";
import { Wrench } from "@flame/icons/react";
import {
  EmptyState,
  ListRow,
  RecordBody,
  RecordGroup,
  RecordHeading,
  Switch,
} from "@flame/ui";
import { serverKey } from "./model";
import { toolPolicyKey, type MCPSettings } from "./settings";
import { CommandFeedback, ReadStatus } from "./feedback";
const column = {
  display: "grid",
  minWidth: 0,
  gap: "var(--layout-content-gap)",
} as const;
const controls = {
  display: "flex",
  flexWrap: "wrap",
  alignItems: "center",
  justifyContent: "flex-end",
  gap: "var(--layout-content-gap)",
  minWidth: 0,
} as const;
const choice = {
  display: "inline-flex",
  alignItems: "center",
  gap: "var(--layout-control-gap)",
  minWidth: 0,
  maxWidth: "100%",
} as const;
function toolSource(ref: ToolRef) {
  return ref.type === "builtIn"
    ? `builtIn/${ref.name}`
    : `mcp/${ref.server.origin.type === "installation" ? `${ref.server.origin.installationId}/` : ""}${ref.server.name}/${ref.name}`;
}
export function ToolControls({
  context,
  settings,
  server,
  t,
}: {
  context: ComponentContext;
  settings: MCPSettings;
  server: MCPServerID;
  t: Translate;
}) {
  const state = useSyncExternalStore(settings.subscribe, settings.getSnapshot);
  const key = serverKey(server);
  const load = useEffectEvent(() => settings.readTools(context, server));
  useEffect(() => load(), [settings, context.signal, key]);
  const tools = state.tools.get(key);
  const exposure = state.exposure.get(key);
  const rules = state.rules;
  const disabled = new Set(exposure?.data);
  const busy = settings.busy(key) || state.probing.has(key);
  const policy = toolPolicyKey(server);
  const command = state.commands.get(policy);
  return (
    <div style={column}>
      <RecordBody>{t("mcp.tools.policyHint")}</RecordBody>
      {command?.phase === "running" && <p role="status">{t("mcp.saving")}</p>}
      <CommandFeedback
        context={context}
        settings={settings}
        resource={policy}
        t={t}
      />
      <ReadStatus
        read={exposure}
        title={t("mcp.error.loadToolVisibility")}
        t={t}
        retry={() => settings.readTools(context, server)}
      />
      <ReadStatus
        read={rules}
        title={t("mcp.error.loadAutoApproveRules")}
        t={t}
        retry={() => settings.readRules(context)}
      />
      <ReadStatus
        read={tools}
        title={t("mcp.error.loadTools")}
        t={t}
        retry={() => settings.readTools(context, server)}
      />
      {tools?.data !== undefined &&
        (tools.data.length ? (
          <RecordGroup variant="list">
            {tools.data.map((tool) => (
              <ListRow key={tool.name} data-focus-unit="">
                <RecordHeading
                  title={tool.name}
                  description={
                    <>
                      {tool.description}
                      {tool.nameConflicts.length > 0 && (
                        <span style={{ display: "block" }}>
                          {t("mcp.tools.nameConflict", {
                            name: tool.modelName,
                            sources: tool.nameConflicts
                              .map(toolSource)
                              .join(", "),
                          })}
                        </span>
                      )}
                    </>
                  }
                  actions={
                    <div style={controls}>
                      <div style={choice}>
                        <span>{t("mcp.tools.enabled")}</span>
                        <Switch
                          checked={!disabled.has(tool.name)}
                          disabled={busy || exposure?.phase !== "ready"}
                          onCheckedChange={(on) =>
                            void settings.setExposure(
                              context,
                              server,
                              tool.name,
                              !on,
                            )
                          }
                          ariaLabel={t("mcp.tools.enable.aria", {
                            tool: tool.name,
                          })}
                        />
                      </div>
                      <div style={choice}>
                        <span>{t("mcp.tools.autoApprove")}</span>
                        <Switch
                          checked={settings
                            .allowRules(server, tool.name)
                            .some((rule) => !rule.stale)}
                          disabled={busy || rules.phase !== "ready"}
                          onCheckedChange={(on) =>
                            void settings.setAutoApprove(
                              context,
                              server,
                              tool.name,
                              on,
                            )
                          }
                          ariaLabel={t("mcp.tools.autoApprove.aria", {
                            tool: tool.name,
                          })}
                        />
                      </div>
                    </div>
                  }
                />
              </ListRow>
            ))}
          </RecordGroup>
        ) : (
          <EmptyState icon={Wrench} title={t("mcp.tools.empty")} />
        ))}
    </div>
  );
}
