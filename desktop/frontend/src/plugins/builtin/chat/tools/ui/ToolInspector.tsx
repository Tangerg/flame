import * as stylex from "@stylexjs/stylex";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { SectionLabel, Well } from "@/ui";
import { useT } from "@/lib/i18n";
import { toolInspectorModel, type ToolInspectorBody } from "../application/toolInspectorModel";
import { face, space, type as typeStep } from "@/styles/tokens.stylex";
import { chatStyles as ct } from "../../chatStyles";

const ti = stylex.create({
  body: { paddingTop: space.s0_5 },
  section: { marginBottom: { default: space.s2, ":last-child": 0 } },
  label: { paddingInline: 0, paddingTop: 0, paddingBottom: space.s1 },
});

export function ToolInspector({ tool }: { tool: ToolCall }) {
  const t = useT();
  const model = toolInspectorModel(tool);

  return (
    <div {...stylex.props(ti.body)}>
      <InspectorSection title={t("toolInspector.arguments")} body={model.args} />
      {model.result.text && (
        <InspectorSection title={t("toolInspector.result")} body={model.result} />
      )}
      {model.showNoResult && (
        <div {...stylex.props(ct.faint, typeStep.uiSm, face.mono)}>
          {t("toolInspector.noResult")}
        </div>
      )}
    </div>
  );
}

function InspectorSection({ title, body }: { title: string; body: ToolInspectorBody }) {
  if (!body.text) return null;
  return (
    <div {...stylex.props(ti.section)}>
      <SectionLabel
        className={stylex.props(ti.label).className}
        trailing={body.isJson ? <span {...stylex.props(face.mono)}>json</span> : undefined}
      >
        {title}
      </SectionLabel>
      <Well cap="md" wrap={body.isJson ? "pre" : "anywhere"}>
        {body.text}
      </Well>
    </div>
  );
}
