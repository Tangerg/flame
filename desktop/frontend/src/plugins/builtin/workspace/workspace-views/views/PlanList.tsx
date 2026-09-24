import * as stylex from "@stylexjs/stylex";
import { useEffect, useRef } from "react";
import type { PlanStep } from "@/plugins/builtin/agent/public/plan";
import { SectionLabel, StepRow } from "@/ui";
import { useT } from "@/lib/i18n";
import { viewStyles as vs } from "./viewStyles";

export function PlanList({ steps }: { steps: readonly PlanStep[] }) {
  const t = useT();
  const list = useRef<HTMLDivElement>(null);
  const current = steps.find((step) => step.status === "active")?.id;
  useEffect(() => {
    if (current === undefined) return;
    list.current?.querySelector('[aria-current="step"]')?.scrollIntoView({ block: "nearest" });
  }, [current]);
  return (
    <div ref={list} {...stylex.props(vs.gutter, vs.planPad)}>
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
