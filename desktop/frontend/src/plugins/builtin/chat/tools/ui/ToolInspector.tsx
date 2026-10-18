import * as stylex from "@stylexjs/stylex";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { ShikiCodeBlock } from "@/ui";
import { Disclosure, vocab } from "@flame/ui";
import { useT } from "@/lib/i18n";
import { toolInspectorModel, type ToolInspectorBody } from "../application/toolInspectorModel";
import { space, type as typeStep } from "@flame/ui/tokens.stylex";

const ti = stylex.create({
  body: {
    display: "flex",
    flexDirection: "column",
    gap: space.s2,
    minWidth: 0,
    paddingTop: space.s0_5,
  },
  empty: { margin: 0 },
});

const sourceLang = (body: ToolInspectorBody) => (body.isJson ? "json" : "text");

export function ToolInspector({ tool }: { tool: ToolCall }) {
  const t = useT();
  const model = toolInspectorModel(tool);
  const hasResult = model.result.text !== "";

  return (
    <div {...stylex.props(ti.body)}>
      {hasResult && (
        <ShikiCodeBlock
          lang={sourceLang(model.result)}
          code={model.result.text}
          scrollLabel={t("toolInspector.result")}
        />
      )}
      {model.showNoResult && (
        <p {...stylex.props(ti.empty, vocab.faint, typeStep.uiSm)}>{t("toolInspector.noResult")}</p>
      )}
      {model.args.text && (
        <Disclosure title={t("toolInspector.arguments")} defaultOpen={!hasResult}>
          <ShikiCodeBlock
            lang={sourceLang(model.args)}
            code={model.args.text}
            scrollLabel={t("toolInspector.arguments")}
            copyable={false}
          />
        </Disclosure>
      )}
    </div>
  );
}
