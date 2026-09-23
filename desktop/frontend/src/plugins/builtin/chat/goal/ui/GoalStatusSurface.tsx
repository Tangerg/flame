import * as stylex from "@stylexjs/stylex";
import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { useRef, useState } from "react";
import { Button, IconButton, TextEditorDialog, vocab } from "@/ui";
import { AgentComposerTopTraySurface } from "@/ui/agent";
import { color } from "@/styles/tokens.stylex";

const goalTray = stylex.create({
  material: {
    width: "100%",
    marginBottom: "-1px",
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
import { useT } from "@/lib/i18n";
import { rpcErrorText } from "@/lib/rpcErrors";
import { notifyError } from "@/plugins/sdk";
import { clearGoal, resumeGoal, stopGoal, updateGoal } from "../application/goalCommands";
import { GOAL_STATUS_I18N, goalCanResume } from "../application/goalStatusPresentation";
import { type GoalReadModel, useGoalMaterial } from "../application/goalReadModel";
import {
  runtimeCommandsAvailable,
  useRuntimeCommandsAvailable,
} from "@/plugins/builtin/runtime/public/serviceStatus";
import { GoalGlyph } from "./GoalGlyph";
import { space } from "@/styles/tokens.stylex";

const gs = stylex.create({
  bar: {
    display: "flex",
    width: "100%",
    alignItems: "center",
    justifyContent: "space-between",
    gap: space.s2,
    paddingInline: space.s3,
    paddingBlock: space.s1,
  },
  glyph: { height: "var(--icon-sm)", width: "var(--icon-sm)" },
  bigGlyph: { height: "var(--icon-lg)", width: "var(--icon-lg)" },
  summary: {
    minHeight: space.s6,
    textDecorationLine: "underline",
    textDecorationColor: { default: "transparent", ":is(:enabled):hover": "currentColor" },
    cursor: { ':is(:disabled, [aria-disabled="true"])': "default" },
    opacity: { ':is(:disabled, [aria-disabled="true"])': 1 },
  },
  objective: { marginInlineStart: space.s1 },
  actions: {
    display: "flex",
    flexShrink: 0,
    alignItems: "center",
    gap: space.s3,
    marginInlineStart: space.s2,
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
  const [pending, setPending] = useState<"clear" | "status" | "edit" | null>(null);
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(goal.objective);
  const commandInFlight = useRef(false);
  const runtimeAvailable = useRuntimeCommandsAvailable();
  const canChangeStatus = goal.status === "active" || goalCanResume(goal);
  const canEdit = goal.status !== "completing";
  const nextObjective = draft.trim();
  const canSave = canEdit && nextObjective.length > 0 && nextObjective !== goal.objective;

  const runCommand = async (
    kind: "clear" | "status" | "edit",
    command: () => Promise<void>,
    fallback: string,
  ) => {
    if (commandInFlight.current) return false;
    if (!runtimeCommandsAvailable()) {
      notifyError(fallback);
      return false;
    }
    commandInFlight.current = true;
    setPending(kind);
    try {
      await command();
      return true;
    } catch (error) {
      if (!wasGenerationRetired(error)) notifyError(rpcErrorText(error) ?? fallback);
      return false;
    } finally {
      commandInFlight.current = false;
      setPending(null);
    }
  };

  const changeStatus = async () => {
    if (!canChangeStatus) return;
    const fallback = goal.status === "active" ? t("goal.error.pause") : t("goal.error.resume");
    await runCommand(
      "status",
      () => (goal.status === "active" ? stopGoal(goal.sessionId) : resumeGoal(goal.sessionId)),
      fallback,
    );
  };

  const clear = () => runCommand("clear", () => clearGoal(goal.sessionId), t("goal.error.clear"));

  const save = async () => {
    if (!canSave) return;
    const saved = await runCommand(
      "edit",
      () => updateGoal({ sessionId: goal.sessionId, objective: nextObjective }),
      t("goal.error.update"),
    );
    if (saved) setEditing(false);
  };

  const controlsDisabled = pending !== null || !runtimeAvailable;
  const openEditor = () => {
    setDraft(goal.objective);
    setEditing(true);
  };

  return (
    <>
      <div data-slot="goal-status-row" {...stylex.props(gs.bar)}>
        <div {...stylex.props(vocab.line, vocab.fill)}>
          <GoalGlyph className={stylex.props(gs.glyph, vocab.hold, vocab.faint).className} />
          <Button
            type="button"
            data-goal="summary"
            variant="bare"
            size="xs"
            disabled={!canEdit}
            pending={pending !== null}
            flex="fill"
            title={goal.objective}
            className={stylex.props(gs.summary).className}
            onClick={openEditor}
          >
            <span {...stylex.props(vocab.hold, vocab.ink)}>
              {t(GOAL_STATUS_I18N[goal.status].label)}
            </span>
            <span {...stylex.props(gs.objective, vocab.min, vocab.truncate, vocab.muted)}>
              {goal.objective}
            </span>
          </Button>
        </div>
        <div data-slot="goal-actions" {...stylex.props(gs.actions)}>
          <IconButton
            type="button"
            size="xs"
            icon="trash"
            quiet
            title={t("goal.action.clear")}
            disabled={controlsDisabled}
            aria-busy={pending === "clear"}
            onClick={() => void clear()}
          />
          {canChangeStatus && (
            <IconButton
              type="button"
              size="xs"
              icon={goal.status === "active" ? "pause" : "play"}
              quiet
              title={t(goal.status === "active" ? "goal.action.pause" : "goal.action.resume")}
              disabled={controlsDisabled}
              aria-busy={pending === "status"}
              onClick={() => void changeStatus()}
            />
          )}
          {canEdit && (
            <IconButton
              type="button"
              size="xs"
              icon="edit"
              quiet
              title={t("goal.action.edit")}
              disabled={controlsDisabled}
              onClick={openEditor}
            />
          )}
        </div>
      </div>
      <TextEditorDialog
        open={editing}
        onOpenChange={(open) => {
          if (pending !== "edit") setEditing(open);
        }}
        icon={
          <GoalGlyph
            aria-hidden="true"
            className={stylex.props(gs.bigGlyph, vocab.muted).className}
          />
        }
        title={t("goal.edit.title")}
        closeLabel={t("common.close")}
        label={t("goal.edit.label")}
        value={draft}
        onChange={setDraft}
        cancelLabel={t("common.cancel")}
        saveLabel={t("goal.edit.save")}
        savingLabel={t("goal.edit.saving")}
        busy={pending === "edit"}
        saveDisabled={!canSave}
        onSave={() => void save()}
      />
    </>
  );
}
