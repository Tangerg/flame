import { useNotifications } from "@/features/notifications/public/notifier";
import { isErrorType } from "@flame/runtime-client";
import { useGoalAuthorAvailable } from "@/features/plugins/public/workflows";
import { useRuntime } from "@/features/connection/public/runtime";
import { tupleKey } from "@/lib/tupleKey";
import { iconGlyph } from "@/ui/icons/index";
import * as stylex from "@stylexjs/stylex";
import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { useCallback, useRef, useState } from "react";
import { Icon } from "@/ui/index";
import { TextEditorDialog } from "@/ui/index";
import {
  Button,
  Disclosure,
  IconButton,
  Metadata,
  ModalDialog,
  ScrollRegion,
  vocab,
} from "@flame/ui";
import { AgentComposerTopTraySurface } from "@/ui/agent/index";
import { color, leading, space, type as typeStep } from "@flame/ui/tokens.stylex";
import { useT } from "@/lib/i18n/index";
import { durationText } from "@/lib/format";
import { useElapsedMillis } from "@/lib/useElapsedMillis";
import { useCommandAction } from "@/features/command/public/commandAction";
import { useComposer } from "@/features/composer/public/composer";
import { focusComposer } from "@/features/composer/public/focus";
import { useGoalCommands } from "@/features/goal/public/goals";
import {
  GOAL_STATUS_I18N,
  goalReasonLabel,
  goalCanChangeStatus,
  goalCanEdit,
  goalCreatedAtMillis,
  goalIsActive,
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
  identity: { minWidth: 0, flex: "1 1 13rem" },
  summary: {
    flexWrap: "wrap",
    justifyContent: "flex-start",
    gap: `0 ${space.s2}`,
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
    whiteSpace: "pre-wrap",
    overflowWrap: "anywhere",
  },
  objectiveReader: {
    maxHeight: "40dvh",
    whiteSpace: "pre-wrap",
    overflowWrap: "anywhere",
    lineHeight: leading.body,
  },
  conflictReader: { maxHeight: "20dvh" },
});

export function GoalStatusSurface() {
  const material = useGoalMaterial();
  const goal = material.value?.goal;
  const writable = useGoalAuthorAvailable(goal?.author) && material.value?.available === true;

  if (!goal) return null;
  return (
    <AgentComposerTopTraySurface
      key={tupleKey(goal.sessionId, material.generation.toString(), goal.createdAt)}
      className={stylex.props(goalTray.material).className}
    >
      <GoalRow key={writable ? "writable" : "read-only"} goal={goal} writable={writable} />
    </AgentComposerTopTraySurface>
  );
}

function GoalRow({ goal, writable }: { goal: GoalReadModel; writable: boolean }) {
  const { messages } = useNotifications();
  const runtimeAccess = useRuntime();

  const t = useT();
  const { clear: clearGoal, resume: resumeGoal, stop: stopGoal } = useGoalCommands();
  const composerAccess = useComposer();
  // The row retires with its goal (cleared, settled or withdrawn); focus it
  // held continues in the composer instead of falling to the document.
  const rowRef = useCallback(
    (row: HTMLDivElement | null) => {
      if (!row) return;
      return () => {
        if (row.contains(row.ownerDocument.activeElement)) focusComposer(composerAccess);
      };
    },
    [composerAccess],
  );
  const readerRef = useRef<HTMLDivElement>(null);
  const [readerOpen, setReaderOpen] = useState(false);
  const runtimeAvailable = useRuntimeCommandsAvailable();
  const active = goalIsActive(goal);
  const canChangeStatus = goalCanChangeStatus(goal);
  const canEdit = writable && goalCanEdit(goal);
  const editor = useGoalEditor(goal, canEdit);
  const clearAction = useCommandAction({
    wasRetired: wasGenerationRetired,
    fallback: t("goal.error.clear"),
  });
  const statusFallback = t(active ? "goal.error.pause" : "goal.error.resume");
  const statusAction = useCommandAction({
    wasRetired: wasGenerationRetired,
    fallback: statusFallback,
  });
  const busy = clearAction.busy || statusAction.busy || editor.busy;
  const controlsDisabled = busy || !runtimeAvailable;

  const admitted = (fallback: string) => {
    if (runtimeCommandsAvailable(runtimeAccess)) return true;
    messages.notify("error", fallback);
    return false;
  };

  const changeStatus = () => {
    if (!canChangeStatus || !admitted(statusFallback)) return;
    statusAction.run(() => (active ? stopGoal(goal.sessionId) : resumeGoal(goal.sessionId)));
  };

  const clear = () => {
    if (!admitted(t("goal.error.clear"))) return;
    clearAction.run(() => clearGoal(goal.sessionId));
  };

  return (
    <>
      <div ref={rowRef} data-slot="goal-status-row" {...stylex.props(gs.bar)}>
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
            disabled={writable && !canEdit}
            pending={busy}
            flex="fill"
            title={goal.objective}
            styles={gs.summary}
            onClick={writable ? editor.begin : () => setReaderOpen(true)}
          >
            <span {...stylex.props(vocab.min, vocab.truncate, vocab.ink, typeStep.uiSm)}>
              {goal.objective}
            </span>
            {goal.status === "active" ? (
              <span className="sr-only">{t(GOAL_STATUS_I18N[goal.status].label)}</span>
            ) : (
              <span {...stylex.props(vocab.muted, typeStep.uiSm)}>
                {t(GOAL_STATUS_I18N[goal.status].label)}
              </span>
            )}
          </Button>
        </div>
        {writable && (
          <div data-slot="goal-actions" {...stylex.props(gs.actions)}>
            <GoalActions
              createdAt={goalCreatedAtMillis(goal.createdAt)}
              disabled={controlsDisabled}
              clearBusy={clearAction.busy}
              onClear={clear}
              status={
                canChangeStatus
                  ? { active, busy: statusAction.busy, onChange: changeStatus }
                  : undefined
              }
              onEdit={canEdit ? editor.begin : undefined}
            />
          </div>
        )}
      </div>
      <GoalStopReason goal={goal} />
      <ModalDialog
        open={readerOpen}
        onOpenChange={setReaderOpen}
        icon={<Icon name="target" size="lg" className={stylex.props(vocab.muted).className} />}
        title={t("goal.edit.label")}
        closeLabel={t("common.close")}
        initialFocus={readerRef}
        bodyRef={readerRef}
        bodyStyles={gs.objectiveReader}
      >
        {goal.objective}
      </ModalDialog>
      <GoalEditorDialog
        editor={editor}
        open={editor.open && canEdit}
        latestObjective={goal.objective}
        disabled={controlsDisabled}
      />
    </>
  );
}

function GoalActions({
  createdAt,
  disabled,
  clearBusy,
  onClear,
  status,
  onEdit,
}: {
  createdAt: number | null;
  disabled: boolean;
  clearBusy: boolean;
  onClear: () => void;
  status: { active: boolean; busy: boolean; onChange: () => void } | undefined;
  onEdit: (() => void) | undefined;
}) {
  const t = useT();
  return (
    <>
      {createdAt !== null && <GoalElapsed createdAt={createdAt} />}
      <IconButton
        type="button"
        size="xs"
        icon={iconGlyph("trash")}
        quiet
        aria-label={t("goal.action.clear")}
        disabled={disabled}
        aria-busy={clearBusy}
        onClick={onClear}
      />
      {status && (
        <IconButton
          type="button"
          size="xs"
          icon={iconGlyph(status.active ? "circle-pause" : "circle-play")}
          quiet
          aria-label={t(status.active ? "goal.action.pause" : "goal.action.resume")}
          disabled={disabled}
          aria-busy={status.busy}
          onClick={status.onChange}
        />
      )}
      {onEdit && (
        <IconButton
          type="button"
          size="xs"
          icon={iconGlyph("edit")}
          quiet
          aria-label={t("goal.action.edit")}
          disabled={disabled}
          onClick={onEdit}
        />
      )}
    </>
  );
}

// A revision conflict stays for the editing session so the latest objective
// remains readable; other failures clear once the draft changes.
function useGoalEditor(goal: GoalReadModel, canEdit: boolean) {
  const runtimeAccess = useRuntime();
  const t = useT();
  const { update: updateGoal } = useGoalCommands();
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState(goal.objective);
  const [failure, setFailure] = useState<string>();
  const [conflict, setConflict] = useState(false);
  const action = useCommandAction({
    wasRetired: wasGenerationRetired,
    fallback: t("goal.error.update"),
    onError: (message, error) => {
      if (isErrorType(error, "revision_conflict")) setConflict(true);
      else setFailure(message);
    },
  });
  const objective = draft.trim();
  const canSave = canEdit && objective.length > 0 && objective !== goal.objective;
  const changeOpen = (next: boolean) => {
    setOpen(next);
    if (!next) {
      setFailure(undefined);
      setConflict(false);
    }
  };
  return {
    open,
    draft,
    conflict,
    error: failure ?? (conflict ? t("goal.edit.conflict") : undefined),
    busy: action.busy,
    canSave,
    changeOpen,
    begin: () => {
      setDraft(goal.objective);
      setFailure(undefined);
      setOpen(true);
    },
    edit: (value: string) => {
      setDraft(value);
      setFailure(undefined);
    },
    save: () => {
      if (!canSave) return;
      if (!runtimeCommandsAvailable(runtimeAccess)) {
        setFailure(t("goal.error.update"));
        return;
      }
      setFailure(undefined);
      action.run(async () => {
        await updateGoal({ sessionId: goal.sessionId, objective });
        changeOpen(false);
      });
    },
  };
}

function GoalEditorDialog({
  editor,
  open,
  latestObjective,
  disabled,
}: {
  editor: ReturnType<typeof useGoalEditor>;
  open: boolean;
  latestObjective: string;
  disabled: boolean;
}) {
  const t = useT();
  const latestObjectiveLabel = t("goal.edit.latestObjective");
  return (
    <TextEditorDialog
      open={open}
      onOpenChange={editor.changeOpen}
      icon={<Icon name="target" size="lg" className={stylex.props(vocab.muted).className} />}
      title={t("goal.edit.title")}
      closeLabel={t("common.close")}
      value={editor.draft}
      onChange={editor.edit}
      cancelLabel={t("common.cancel")}
      saveLabel={t("goal.edit.save")}
      savingLabel={t("goal.edit.saving")}
      busy={editor.busy}
      error={editor.error}
      feedbackBody={
        editor.conflict && (
          <div>
            <Metadata>{latestObjectiveLabel}</Metadata>
            <ScrollRegion
              label={latestObjectiveLabel}
              styles={[gs.objectiveReader, gs.conflictReader]}
              data-focus-inset=""
            >
              {latestObjective}
            </ScrollRegion>
          </div>
        )
      }
      saveDisabled={!editor.canSave || disabled}
      onSave={editor.save}
    />
  );
}

function GoalStopReason({ goal }: { goal: GoalReadModel }) {
  const t = useT();
  if (!goal.reason) return null;
  return (
    <div data-slot="goal-stop-reason" {...stylex.props(gs.reason, vocab.muted)}>
      {goal.reason.explanation ? (
        <Disclosure title={t(goalReasonLabel(goal.reason))} keepMounted>
          <ScrollRegion
            label={t(goalReasonLabel(goal.reason))}
            styles={gs.reasonDetail}
            data-focus-inset=""
          >
            {goal.reason.explanation}
          </ScrollRegion>
        </Disclosure>
      ) : (
        <span>{t(goalReasonLabel(goal.reason))}</span>
      )}
    </div>
  );
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
