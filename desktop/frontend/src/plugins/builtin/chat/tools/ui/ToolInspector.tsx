import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { SectionLabel, Well } from "@/ui";
import { useT } from "@/lib/i18n";
import { toolInspectorModel, type ToolInspectorBody } from "../application/toolInspectorModel";

export function ToolInspector({ tool }: { tool: ToolCall }) {
  const t = useT();
  const model = toolInspectorModel(tool);

  return (
    <div className="pt-0.5">
      <InspectorSection title={t("toolInspector.arguments")} body={model.args} />
      {model.result.text && (
        <InspectorSection title={t("toolInspector.result")} body={model.result} />
      )}
      {model.showNoResult && (
        <div className="font-mono text-ui-sm text-fg-faint">{t("toolInspector.noResult")}</div>
      )}
    </div>
  );
}

function InspectorSection({ title, body }: { title: string; body: ToolInspectorBody }) {
  if (!body.text) return null;
  return (
    <div className="mb-2 last:mb-0">
      <SectionLabel
        className="px-0 pt-0 pb-1"
        trailing={body.isJson ? <span className="font-mono">json</span> : undefined}
      >
        {title}
      </SectionLabel>
      <Well cap="md" wrap={body.isJson ? "pre" : "anywhere"}>
        {body.text}
      </Well>
    </div>
  );
}
