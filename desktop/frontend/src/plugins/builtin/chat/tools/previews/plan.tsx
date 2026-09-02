import type { ToolPreviewProps } from "@/plugins/sdk";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { planStepsFromToolArgs } from "@/plugins/builtin/agent/public/plan";
import { PreviewPlaceholder } from "@/plugins/builtin/chat/tools/public/previews/PreviewPlaceholder";
import { StepRow } from "@/ui";
import { toolPreviews } from "../application/toolPreviewContributions";
import { ToolResultProse } from "./previewChrome";

function SetPlanPreview({ tool }: ToolPreviewProps) {
  const steps = planStepsFromToolArgs(tool.args);
  return (
    <div className="pt-1">
      {steps.length > 0 ? (
        <ol className="flex flex-col">
          {steps.map((step) => (
            <li key={step.id}>
              <StepRow state={step.status}>{step.text}</StepRow>
            </li>
          ))}
        </ol>
      ) : (
        <PreviewPlaceholder
          status={tool.status}
          pending="tools.preview.pending.running"
          idle="tools.preview.idle.empty"
        />
      )}
    </div>
  );
}

export const planPreviews = definePlugin({
  name: "flame.builtin.plan-previews",
  setup(ctx) {
    for (const preview of toolPreviews({
      enter_plan_mode: ToolResultProse,
      set_plan: SetPlanPreview,
      exit_plan_mode: ToolResultProse,
    })) {
      ctx.contribute(TOOL_PREVIEW, preview.component, { key: preview.key });
    }
  },
});
