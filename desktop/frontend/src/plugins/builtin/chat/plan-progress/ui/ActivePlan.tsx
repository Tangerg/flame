import * as stylex from "@stylexjs/stylex";
import { AnimatePresence, motion } from "motion/react";
import { Gauge, Pressable, RichTooltip, StepMark, vocab } from "@/ui";
import { disclosureExitTransition, disclosureTransition } from "@/lib/motion";
import { useT } from "@/lib/i18n";
import { type PlanStep, useSessionPlan } from "@/plugins/builtin/agent/public/plan";
import { activePlanState, type ActivePlanState } from "../application/progress";
import {
  color,
  radius,
  space,
  type as typeStep,
  motion as motionToken,
} from "@/styles/tokens.stylex";

const ap = stylex.create({
  pill: {
    marginBlock: "calc(var(--spacing) * -1.5)",
    display: "inline-flex",
    maxWidth: "100%",
    minWidth: 0,
    alignItems: "center",
    gap: space.s1_5,
    borderRadius: radius.sm,
    paddingBlock: space.s1_5,
    color: { default: color.fgMuted, ":hover": color.fg },
    transitionProperty: "color",
    transitionTimingFunction: motionToken.easeState,
  },
  host: {
    position: "relative",
    height: space.s8,
    width: "100%",
    maxWidth: "100%",
    alignSelf: "flex-end",
  },
  dock: {
    position: "absolute",
    insetInline: 0,
    bottom: space.s1,
    display: "flex",
    minHeight: "calc(var(--spacing) * 7)",
    alignItems: "center",
    justifyContent: "center",
    gap: space.s2,
    paddingBottom: space.s1,
  },
  panel: {
    maxHeight: "min(320px, calc(100vh - 16px))",
    maxWidth: "min(320px, calc(100vw - 16px))",
    overflowY: "auto",
  },
  steps: { display: "flex", flexDirection: "column", gap: space.s2 },
  step: {
    display: "flex",
    maxWidth: "calc(var(--spacing) * 80)",
    minWidth: 0,
    alignItems: "flex-start",
    gap: space.s2,
    lineHeight: "1rem",
  },
  stepText: {
    minWidth: 0,
    maxWidth: "calc(var(--spacing) * 72)",
    overflowWrap: "break-word",
  },
});

export function ActivePlan() {
  const plan = useSessionPlan();
  const progress = activePlanState(plan);

  return (
    <AnimatePresence initial={false}>
      {progress.visible && <PlanPill key={plan.identity} steps={plan.steps} progress={progress} />}
    </AnimatePresence>
  );
}

function PlanPill({ steps, progress }: { steps: readonly PlanStep[]; progress: ActivePlanState }) {
  const t = useT();
  const completionLabel = t("plan.complete", {
    done: progress.done,
    total: progress.total,
  });
  // A plan with no step in flight is either finished or not started; its standing
  // is the completion count, not a position nothing currently occupies.
  const currentIndex = progress.current
    ? steps.findIndex((step) => step.id === progress.current?.id)
    : -1;
  const progressLabel =
    currentIndex < 0
      ? completionLabel
      : t("plan.progress", { current: currentIndex + 1, total: progress.total });

  const trigger = (
    <Pressable
      type="button"
      data-slot="active-plan-pill"
      aria-label={progressLabel}
      className={stylex.props(ap.pill, typeStep.uiSm).className}
    >
      <Gauge
        value={progress.percent / 100}
        label={completionLabel}
        className={stylex.props(vocab.accent).className}
      />
      <span {...stylex.props(vocab.truncate, vocab.figures)}>{progressLabel}</span>
    </Pressable>
  );

  return (
    <motion.div
      initial={{ opacity: 0, y: -4 }}
      animate={{ opacity: 1, y: 0 }}
      exit={{ opacity: 0, y: -4, transition: disclosureExitTransition }}
      transition={disclosureTransition}
      data-slot="active-plan-surface"
      {...stylex.props(ap.host)}
    >
      <div {...stylex.props(ap.dock)}>
        <RichTooltip trigger={trigger} side="top" sideOffset={8} delay={0} styles={ap.panel}>
          <ul {...stylex.props(ap.steps)}>
            {steps.map((step) => (
              <li key={step.id} {...stylex.props(ap.step)}>
                <StepMark state={step.status} />
                <span
                  {...stylex.props(
                    ap.stepText,
                    step.status === "done" ? vocab.muted : vocab.soft,
                    typeStep.uiSm,
                  )}
                >
                  {step.text}
                </span>
              </li>
            ))}
          </ul>
        </RichTooltip>
      </div>
    </motion.div>
  );
}
