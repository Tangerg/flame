import * as stylex from "@stylexjs/stylex";
import type { PlanStep } from "@/plugins/builtin/agent/public/plan";
import { SectionLabel, StepRow } from "@/ui";
import { useT } from "@/lib/i18n";
import { viewStyles as vs } from "./viewStyles";

export function PlanList({ steps }: { steps: readonly PlanStep[] }) {
  const t = useT();
  return (
    <div {...stylex.props(vs.gutter, vs.planPad)}>
      <SectionLabel className={stylex.props(vs.planHeading).className}>
        {t("plan.list.heading")}
      </SectionLabel>
      {steps.map((step) => (
        <StepRow key={step.id} state={step.status}>
          {step.text}
        </StepRow>
      ))}
    </div>
  );
}
