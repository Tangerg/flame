import * as stylex from "@stylexjs/stylex";
import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { useRef, useState } from "react";
import { Button, IconButton, TextEditorDialog, vocab } from "@/ui";
import { AgentComposerTopTraySurface } from "@/ui/agent";
import { color } from "@/styles/tokens.stylex";

// The Goal tray's own material: an edge on three sides and the composer's backdrop, which the
// shared surface used to declare for everyone. Its bottom edge is absent and it overlaps the
// composer by a pixel, because the two are one surface where they meet.
const goalTray = stylex.create({
  material: {
    // The Goal tray spans the composer. Stated here because the composer centres its children
    // and the surface deliberately holds no width — the project tray wants a different one.
    width: "100%",
    marginBottom: "-1px",
    borderTopWidth: "1px",
    borderLeftWidth: "1px",
    borderRightWidth: "1px",
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
import {
  GOAL_STATUS_I18N,
  goalCanResume,
  goalRefusalLabel,
} from "../application/goalStatusPresentation";
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
  // The objective is CONTENT, so when it cannot be edited it keeps its ink and its cursor:
  // the row is telling you what the goal is, not offering a control that is switched off.
  // Three of the four controls on this bar answered the pointer and this one — the widest, and
  // the one that opens the editor — answered nothing. `bare` is right for it: the row reads as
  // content, so it takes no plate and a wash would give it one. What it can say is the same
  // thing `TextButton` says for interactive text, and only while it IS editable, because the
  // comment below is the rule this row is held to.
  summary: {
    minHeight: space.s6,
    textDecorationLine: "underline",
    textDecorationColor: { default: "transparent", ":is(:enabled):hover": "currentColor" },
    cursor: { ":disabled": "default" },
    opacity: { ":disabled": 1 },
  },
  objective: { marginInlineStart: space.s1 },
  // WCAG 2.5.8 lets a target under 24px pass on SPACING, which is how the 22px control step
  // clears it everywhere else. Three of them at 8px beside a full-width summary target did
  // not: axe measured 15.6px and 21.6px of safe clickable space against the 24px it needs.
  // The step stays 22px — it is not the thing that is wrong — and this row gives it room.
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
  const canSave = nextObjective.length > 0 && nextObjective !== goal.objective;

  const runCommand = async (
    kind: "clear" | "status" | "edit",
    command: () => Promise<void>,
    fallback: string,
  ) => {
    if (commandInFlight.current) return false;
    // A runtime that will not take commands is a FAILURE to report, not a reason to go quiet.
    // The three icon controls are disabled while it is away, but the objective itself stays
    // live — it is content — so the editor it opens is reachable, and its Save used to return
    // here and do nothing at all: no write, no error, and a dialog that stayed open.
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
    if (!canEdit || !canSave) return;
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
            // The objective is the one thing on this row with no room: measured at 155px in a
            // 480px string, so two thirds of what the agent is pursuing was unreadable and the
            // only way to see it was to open the editor.
            title={goal.objective}
            className={stylex.props(gs.summary).className}
            onClick={openEditor}
          >
            <span {...stylex.props(vocab.hold, vocab.ink)}>
              {t(goalRefusalLabel(goal) ?? GOAL_STATUS_I18N[goal.status].label)}
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
            iconSize="xs"
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
              iconSize="xs"
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
              iconSize="xs"
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
