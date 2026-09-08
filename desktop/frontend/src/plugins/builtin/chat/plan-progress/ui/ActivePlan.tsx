import * as stylex from "@stylexjs/stylex";
import { AnimatePresence, motion } from "motion/react";
import { Gauge, Pressable, RichTooltip, StepMark } from "@/ui";
import { disclosureTransition } from "@/lib/motion";
import { useT } from "@/lib/i18n";
import { type PlanStep, useSessionPlan } from "@/plugins/builtin/agent/public/plan";
import { useIsCurrentRootRunning } from "@/plugins/builtin/agent/public/run";
import { activePlanState, type ActivePlanState } from "../application/progress";
import { color, radius, space, type as typeStep } from "@/styles/tokens.stylex";
import { chatStyles as ct } from "../../chatStyles";

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
  },
  // The pill floats over the composer's top edge, so the row holds its own measure and the
  // pill docks to the bottom of it — the surface above must not resize as the plan advances.
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
  },
  // A step wraps rather than truncating: half a step is not a step. Its ink comes from the
  // tooltip's own vocabulary — `text-on-fg` is the INVERTED ink, for a plate filled with the
  // foreground colour, and on this surface it rendered at 1.00:1 in both themes.
  stepText: {
    minWidth: 0,
    maxWidth: "calc(var(--spacing) * 72)",
    overflowWrap: "break-word",
    lineHeight: "1rem",
  },
});

export function ActivePlan() {
  const plan = useSessionPlan();
  const progress = activePlanState(plan, useIsCurrentRootRunning());

  return (
    <AnimatePresence initial={false}>
      {progress.visible && progress.current && (
        <PlanPill
          key={plan.identity}
          steps={plan.steps}
          progress={progress}
          current={progress.current}
        />
      )}
    </AnimatePresence>
  );
}

function PlanPill({
  steps,
  progress,
  current,
}: {
  steps: readonly PlanStep[];
  progress: ActivePlanState;
  current: PlanStep;
}) {
  const t = useT();
  const currentIndex = Math.max(
    0,
    steps.findIndex((step) => step.id === current.id),
  );
  const progressLabel = t("plan.progress", {
    current: currentIndex + 1,
    total: progress.total,
  });
  const completionLabel = t("plan.complete", {
    done: progress.done,
    total: progress.total,
  });

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
        className={stylex.props(ct.accent).className}
      />
      <span {...stylex.props(ct.truncate, ct.figures)}>{progressLabel}</span>
    </Pressable>
  );

  return (
    <motion.div
      initial={{ opacity: 0, y: -4 }}
      animate={{ opacity: 1, y: 0 }}
      exit={{ opacity: 0, y: -4 }}
      transition={disclosureTransition}
      data-slot="active-plan-surface"
      {...stylex.props(ap.host)}
    >
      <div {...stylex.props(ap.dock)}>
        <RichTooltip
          trigger={trigger}
          side="top"
          sideOffset={8}
          delay={0}
          className={stylex.props(ap.panel).className}
        >
          <ul {...stylex.props(ap.steps)}>
            {steps.map((step) => (
              <li key={step.id} {...stylex.props(ap.step)}>
                <StepMark state={step.status} />
                <span
                  {...stylex.props(
                    ap.stepText,
                    step.status === "done" ? ct.muted : ct.soft,
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
