import { iconGlyph } from "@/ui/icons/index";
import * as stylex from "@stylexjs/stylex";
import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { useState } from "react";
import { Icon } from "@/ui/index";
import { TextEditorDialog } from "@/ui/index";
import { Button, Disclosure, IconButton, vocab } from "@flame/ui";
import { AgentComposerTopTraySurface } from "@/ui/agent/index";
import { color, space, type as typeStep } from "@flame/ui/tokens.stylex";
import { useT } from "@/lib/i18n/index";
import { durationText } from "@/lib/format";
import { useElapsedMillis } from "@/lib/useElapsedMillis";
import { notifyError } from "@/features/notifications/public/notifications";
import { useCommandAction } from "@/features/command/public/commandAction";
import {
  clearGoal,
  resumeGoal,
  stopGoal,
  updateGoal,
} from "@/features/goal/application/goalCommands";
import {
  GOAL_STATUS_I18N,
  GOAL_REASON_I18N,
  goalCanResume,
} from "@/features/goal/application/goalStatusPresentation";
import { type GoalReadModel, useGoalMaterial } from "@/features/goal/application/goalReadModel";
import {
  runtimeCommandsAvailable,
  useRuntimeCommandsAvailable,
} from "@/features/connection/public/serviceStatus";
const goalTray = stylex.create({
  material: {
    alignSelf: "stretch",
    marginInline: space.s3,
    marginBottom: `calc(-1 * ${space.s3})`,
    paddingBottom: space.s3,
    borderTopWidth: "var(--control-edge-width)",
    borderLeftWidth: "var(--control-edge-width)",
    borderRightWidth: "var(--control-edge-width)",
    borderBottomWidth: 0,
    borderStyle: "solid",
    borderColor: "var(--composer-tray-edge-color)",
    backgroundColor: "var(--app-composer-tray-surface)",
    color: color.fg,
    WebkitBackdropFilter: "var(--composer-tray-backdrop)",
    backdropFilter: "var(--composer-tray-backdrop)",
  },
});

const gs = stylex.create({
  bar: {
    display: "flex",
    flexWrap: "wrap",
    width: "100%",
    alignItems: "center",
    justifyContent: "space-between",
    gap: space.s2,
    paddingInline: space.s3,
    paddingBlock: space.s1,
  },
  identity: { minWidth: 0, flex: "1 1 24ch" },
  summary: {
    flexWrap: "nowrap",
    justifyContent: "flex-start",
    gap: space.s2,
    minHeight: "max(calc(var(--spacing) * 6), var(--control-target-min))",
    textDecorationLine: "underline",
    textDecorationColor: {
      default: "transparent",
      ":hover:not(:disabled, [aria-disabled=true], [data-disabled])": "currentColor",
    },
    cursor: { default: null, ':is(:disabled, [aria-disabled="true"])': "default" },
    opacity: { default: null, ':is(:disabled, [aria-disabled="true"])': 1 },
  },
  actions: {
    display: "flex",
    flexShrink: 0,
    alignItems: "center",
    gap: space.s2,
    marginInlineStart: "auto",
  },
  reason: {
    display: "grid",
    gridTemplateColumns: "minmax(0,1fr)",
    gap: space.s1,
    paddingInline: space.s3,
    paddingBottom: space.s2,
  },
  reasonDetail: {
    maxHeight: "20vh",
    overflow: "auto",
    whiteSpace: "pre-wrap",
    overflowWrap: "anywhere",
  },
});

export function GoalStatusSurface() {
  const material = useGoalMaterial();
  const goal = material.value?.goal;

  if (!goal) return null;
  return (
    <AgentComposerTopTraySurface
      key={JSON.stringify([goal.sessionId, material.generation.toString(), goal.createdAt])}
      className={stylex.props(goalTray.material).className}
    >
      <GoalRow goal={goal} />
    </AgentComposerTopTraySurface>
  );
}

function GoalRow({ goal }: { goal: GoalReadModel }) {
  const t = useT();
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(goal.objective);
  const runtimeAvailable = useRuntimeCommandsAvailable();
  const canChangeStatus = goal.status === "active" || goalCanResume(goal);
  const canEdit = goal.status !== "completing";
  const nextObjective = draft.trim();
  const canSave = canEdit && nextObjective.length > 0 && nextObjective !== goal.objective;
  const clearAction = useCommandAction({
    wasRetired: wasGenerationRetired,
    fallback: t("goal.error.clear"),
  });
  const statusFallback = t(goal.status === "active" ? "goal.error.pause" : "goal.error.resume");
  const statusAction = useCommandAction({
    wasRetired: wasGenerationRetired,
    fallback: statusFallback,
  });
  const editAction = useCommandAction({
    wasRetired: wasGenerationRetired,
    fallback: t("goal.error.update"),
  });
  const busy = clearAction.busy || statusAction.busy || editAction.busy;
  const createdAt = goalCreatedAtMillis(goal.createdAt);

  const admitted = (fallback: string) => {
    if (runtimeCommandsAvailable()) return true;
    notifyError(fallback);
    return false;
  };

  const changeStatus = () => {
    if (!canChangeStatus || !admitted(statusFallback)) return;
    statusAction.run(() =>
      goal.status === "active" ? stopGoal(goal.sessionId) : resumeGoal(goal.sessionId),
    );
  };

  const clear = () => {
    if (!admitted(t("goal.error.clear"))) return;
    clearAction.run(() => clearGoal(goal.sessionId));
  };

  const save = () => {
    if (!canSave || !admitted(t("goal.error.update"))) return;
    editAction.run(async () => {
      await updateGoal({ sessionId: goal.sessionId, objective: nextObjective });
      setEditing(false);
    });
  };

  const controlsDisabled = busy || !runtimeAvailable;
  const openEditor = () => {
    setDraft(goal.objective);
    setEditing(true);
  };

  return (
    <>
      <div data-slot="goal-status-row" {...stylex.props(gs.bar)}>
        <div data-slot="goal-identity" {...stylex.props(vocab.line, gs.identity)}>
          <Icon
            name="target"
            size="sm"
            className={stylex.props(vocab.hold, vocab.faint).className}
          />
          <Button
            type="button"
            data-goal="summary"
            variant="bare"
            size="xs"
            disabled={!canEdit}
            pending={busy}
            flex="fill"
            title={goal.objective}
            styles={gs.summary}
            onClick={openEditor}
          >
            <span {...stylex.props(vocab.hold, vocab.ink, vocab.strong, typeStep.uiSm)}>
              {t(GOAL_STATUS_I18N[goal.status].label)}
            </span>
            <span {...stylex.props(vocab.fill, vocab.truncate, vocab.muted, typeStep.uiSm)}>
              {goal.objective}
            </span>
          </Button>
        </div>
        <div data-slot="goal-actions" {...stylex.props(gs.actions)}>
          {createdAt !== null && <GoalElapsed createdAt={createdAt} />}
          <IconButton
            type="button"
            size="xs"
            icon={iconGlyph("trash")}
            quiet
            aria-label={t("goal.action.clear")}
            disabled={controlsDisabled}
            aria-busy={clearAction.busy}
            onClick={clear}
          />
          {canChangeStatus && (
            <IconButton
              type="button"
              size="xs"
              icon={iconGlyph(goal.status === "active" ? "circle-pause" : "circle-play")}
              quiet
              aria-label={t(goal.status === "active" ? "goal.action.pause" : "goal.action.resume")}
              disabled={controlsDisabled}
              aria-busy={statusAction.busy}
              onClick={changeStatus}
            />
          )}
          {canEdit && (
            <IconButton
              type="button"
              size="xs"
              icon={iconGlyph("maximize")}
              quiet
              aria-label={t("goal.action.edit")}
              disabled={controlsDisabled}
              onClick={openEditor}
            />
          )}
        </div>
      </div>
      {goal.reason && (
        <div data-slot="goal-stop-reason" {...stylex.props(gs.reason, vocab.muted)}>
          {goal.reason.detail ? (
            <Disclosure title={t(GOAL_REASON_I18N[goal.reason.code].label)}>
              <div {...stylex.props(gs.reasonDetail)}>{goal.reason.detail}</div>
            </Disclosure>
          ) : (
            <span>{t(GOAL_REASON_I18N[goal.reason.code].label)}</span>
          )}
        </div>
      )}
      <TextEditorDialog
        open={editing}
        onOpenChange={(open) => {
          if (!editAction.busy) setEditing(open);
        }}
        icon={<Icon name="target" size="lg" className={stylex.props(vocab.muted).className} />}
        title={t("goal.edit.title")}
        closeLabel={t("common.close")}
        label={t("goal.edit.label")}
        value={draft}
        onChange={setDraft}
        cancelLabel={t("common.cancel")}
        saveLabel={t("goal.edit.save")}
        savingLabel={t("goal.edit.saving")}
        busy={editAction.busy}
        saveDisabled={!canSave}
        onSave={save}
      />
    </>
  );
}

function goalCreatedAtMillis(createdAt: string): number | null {
  const millis = Date.parse(createdAt);
  return Number.isNaN(millis) ? null : millis;
}

function GoalElapsed({ createdAt }: { createdAt: number }) {
  const t = useT();
  const elapsed = useElapsedMillis(createdAt);
  const duration = durationText(t, createdAt, createdAt + elapsed);
  const label = t("goal.elapsed", { duration });
  return (
    <span
      data-slot="goal-elapsed"
      title={label}
      {...stylex.props(vocab.hold, vocab.figures, vocab.muted, typeStep.uiSm)}
    >
      <span aria-hidden="true">{duration}</span>
      <span className="sr-only">{label}</span>
    </span>
  );
}
