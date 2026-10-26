import * as stylex from "@stylexjs/stylex";
import { type ReactNode } from "react";
import { type ToolCall } from "@/features/agent/public/sessionView";
import { ShikiCodeBlock } from "@/ui/index";
import { Disclosure, vocab } from "@flame/ui";
import { useT } from "@/lib/i18n/index";
import {
  toolInspectorModel,
  type ToolInspectorBody,
} from "@/features/chat/tools/application/toolInspectorModel";
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

export function ToolInspector({
  tool,
  children,
}: {
  tool: ToolCall;
  children?: (fallback: ReactNode) => ReactNode;
}) {
  const t = useT();
  const model = toolInspectorModel(tool);
  const hasResult = model.result.text !== "";

  const result = (
    <>
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
    </>
  );

  return (
    <div {...stylex.props(ti.body)}>
      {children ? children(result) : result}
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
