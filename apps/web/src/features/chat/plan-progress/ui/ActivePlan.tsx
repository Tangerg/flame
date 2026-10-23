import * as stylex from "@stylexjs/stylex";
import { AnimatePresence, motion, useIsPresent } from "motion/react";
import { useRef, useState } from "react";
import { Gauge } from "@/ui/index";
import { Popover } from "@/ui/index";
import { Pressable } from "@/ui/index";
import { StepMark } from "@/ui/index";
import { useContentVisible } from "@/ui/index";
import { ScrollRegion, vocab } from "@flame/ui";
import { disclosureExitTransition, disclosureTransition } from "@/lib/motion";
import { useT } from "@/lib/i18n/index";
import { type PlanStep } from "@/features/agent/public/plan";
import { useSessionPlan } from "@/features/agent/public/plan";
import {
  activePlanState,
  type ActivePlanState,
} from "@/features/chat/plan-progress/application/progress";
import {
  color,
  leading,
  radius,
  space,
  type as typeStep,
  motion as motionToken,
} from "@flame/ui/tokens.stylex";
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
    transitionDuration: motionToken.color,
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
    display: "flex",
    flexDirection: "column",
    maxHeight: "min(calc(var(--spacing) * 80), var(--available-height))",
    maxWidth: "min(calc(var(--spacing) * 80), var(--available-width))",
  },
  reader: {
    minHeight: 0,
    overscrollBehavior: "contain",
    paddingInline: space.s3,
    paddingBlock: space.s1_5,
    scrollPaddingBlock: space.s1_5,
  },
  steps: { display: "flex", flexDirection: "column", gap: space.s2 },
  step: {
    display: "flex",
    maxWidth: "calc(var(--spacing) * 80)",
    minWidth: 0,
    alignItems: "flex-start",
    gap: space.s2,
    lineHeight: leading.snug,
  },
  stepText: {
    minWidth: 0,
    maxWidth: "calc(var(--spacing) * 72)",
    overflowWrap: "break-word",
  },
});

export function ActivePlan() {
  const plan = useSessionPlan();
  const visible = useContentVisible();
  const progress = activePlanState(plan);

  if (!visible) return null;

  return (
    <AnimatePresence initial={false}>
      {progress.visible && <PlanPill key={plan.identity} steps={plan.steps} progress={progress} />}
    </AnimatePresence>
  );
}

function PlanPill({ steps, progress }: { steps: readonly PlanStep[]; progress: ActivePlanState }) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const present = useIsPresent();
  const readerRef = useRef<HTMLDivElement>(null);
  const completionLabel = t("plan.complete", {
    done: progress.done,
    total: progress.total,
  });
  const progressLabel =
    progress.currentIndex < 0
      ? completionLabel
      : t("plan.progress", { current: progress.currentIndex + 1, total: progress.total });

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
        <Popover.Root open={open && present} onOpenChange={setOpen}>
          <Popover.Trigger
            render={
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
            }
          />
          <Popover.Content
            side="top"
            sideOffset={8}
            aria-label={t("tools.family.plan")}
            initialFocus={readerRef}
            className={stylex.props(ap.panel).className}
          >
            <ScrollRegion
              ref={readerRef}
              label={completionLabel}
              data-slot="active-plan-reader"
              data-focus-inset=""
              styles={ap.reader}
            >
              <ul {...stylex.props(ap.steps)}>
                {steps.map((step) => (
                  <li
                    key={step.id}
                    aria-current={step.id === progress.current?.id ? "step" : undefined}
                    {...stylex.props(ap.step)}
                  >
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
            </ScrollRegion>
          </Popover.Content>
        </Popover.Root>
      </div>
    </motion.div>
  );
}
