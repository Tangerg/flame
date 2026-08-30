import type { PlanStep } from "@/plugins/builtin/agent/public/plan";
import { SectionLabel, StepRow } from "@/ui";
import { useT } from "@/lib/i18n";

export function PlanList({ steps }: { steps: readonly PlanStep[] }) {
  const t = useT();
  return (
    <div className="px-4.5 py-3.5">
      <SectionLabel className="px-0 pt-0">{t("plan.list.heading")}</SectionLabel>
      {steps.map((step) => (
        <StepRow key={step.id} state={step.status}>
          {step.text}
        </StepRow>
      ))}
    </div>
  );
}
