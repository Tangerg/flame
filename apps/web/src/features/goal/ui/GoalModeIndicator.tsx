import * as stylex from "@stylexjs/stylex";
import { useEffect, useSyncExternalStore } from "react";
import { ConfirmDialog } from "@/ui/index";
import { Icon } from "@/ui/index";
import { Button, GlyphSwap, vocab } from "@flame/ui";
import { useT } from "@/lib/i18n/index";
import { useActiveSessionId } from "@/features/agent/public/session";
import { GOAL_SLASH_COMMAND } from "@/features/goal/application/goalReadModel";
import { useGoalAccess } from "@/features/goal/public/goals";
import { useComposer } from "@/features/composer/public/composer";
import { parseComposerSlashCommand } from "@/features/composer/public/draft";
import { captureComposerFocus } from "@/features/composer/public/focus";
export function GoalModeIndicator() {
  const sessionId = useActiveSessionId();
  if (!sessionId) return null;
  return <SessionGoalModeIndicator key={sessionId} sessionId={sessionId} />;
}

function SessionGoalModeIndicator({ sessionId }: { sessionId: string }) {
  const t = useT();
  const composer = useComposer();
  const owner = useGoalAccess().submissions;
  const readSubmission = () => owner.snapshot(sessionId);
  const snapshot = useSyncExternalStore(owner.subscribe, readSubmission, readSubmission);
  const slash = parseComposerSlashCommand(composer.useText());

  useEffect(
    () => () => {
      owner.cancelReplacement(sessionId);
    },
    [owner, sessionId],
  );

  if (slash?.command !== GOAL_SLASH_COMMAND) return null;

  return (
    <>
      <Button
        type="button"
        variant="ghost"
        size="md"
        aria-label={t("goal.mode.clear")}
        disabled={snapshot?.phase === "starting"}
        chip
        onClick={() => {
          const restoreFocus = captureComposerFocus(composer);
          if (composer.editLeadingSlashCommand(GOAL_SLASH_COMMAND, "remove")) restoreFocus();
        }}
      >
        <GlyphSwap
          rest={
            <Icon name="target" size="composer" className={stylex.props(vocab.hold).className} />
          }
          hover={<Icon name="x" size="composer" className={stylex.props(vocab.hold).className} />}
        />
        <span>{t("goal.mode.label")}</span>
      </Button>
      {snapshot?.phase === "confirming" && (
        <ConfirmDialog
          open
          onDismiss={() => owner.cancelReplacement(sessionId)}
          title={t("goal.replace.title")}
          body={t("goal.replace.body", { objective: snapshot.replacedObjective })}
          confirmLabel={t("goal.replace.confirm")}
          cancelLabel={t("common.cancel")}
          destructive
          onConfirm={() => owner.confirmReplacement(sessionId)}
        />
      )}
    </>
  );
}
