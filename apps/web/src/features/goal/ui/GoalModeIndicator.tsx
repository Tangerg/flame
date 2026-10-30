import * as stylex from "@stylexjs/stylex";
import { useEffect, useSyncExternalStore } from "react";
import { ConfirmDialog } from "@/ui/index";
import { Icon } from "@/ui/index";
import { Button, GlyphSwap, vocab } from "@flame/ui";
import { useT } from "@/lib/i18n/index";
import { useActiveSessionId } from "@/features/agent/public/session";
import { useGoalMaterial } from "@/features/goal/application/goalReadModel";
import { useGoalAccess } from "@/features/goal/public/goals";
import { goalCanEnterComposerMode } from "@/features/goal/application/goalComposerSubmitMode";
import { useComposer } from "@/features/composer/public/composer";
import { captureComposerFocus } from "@/features/composer/public/focus";
export function GoalModeIndicator() {
  const sessionId = useActiveSessionId();
  if (!sessionId) return null;
  return <SessionGoalModeIndicator key={sessionId} sessionId={sessionId} />;
}

function SessionGoalModeIndicator({ sessionId }: { sessionId: string }) {
  const t = useT();
  const composer = useComposer();
  const owner = useGoalAccess().composerMode;
  const snapshot = useSyncExternalStore(owner.subscribe, owner.snapshot, owner.snapshot);
  const material = useGoalMaterial().value;
  const goal = material?.goal ?? null;
  const available = material?.available === true && goalCanEnterComposerMode(goal);
  const active = snapshot.sessionId === sessionId && snapshot.phase !== "inactive";

  useEffect(
    () => () => {
      owner.deactivate(sessionId);
    },
    [owner, sessionId],
  );

  useEffect(() => {
    if (!available && snapshot.sessionId === sessionId && snapshot.phase !== "starting") {
      owner.deactivate(sessionId);
    }
  }, [available, owner, sessionId, snapshot.phase, snapshot.sessionId]);

  if (!active) return null;
  const confirming = snapshot.phase === "confirming";
  const starting = snapshot.phase === "starting";

  return (
    <>
      <Button
        type="button"
        variant="ghost"
        size="md"
        aria-label={t("goal.mode.clear")}
        disabled={starting}
        chip
        onClick={() => {
          const restoreFocus = captureComposerFocus(composer);
          if (owner.deactivate(sessionId)) restoreFocus();
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
      <ConfirmDialog
        open={confirming}
        onOpenChange={(open) => {
          if (!open) owner.cancelReplacement(sessionId);
        }}
        title={t("goal.replace.title")}
        body={t("goal.replace.body", { objective: snapshot.replacedObjective ?? "" })}
        confirmLabel={t("goal.replace.confirm")}
        cancelLabel={t("common.cancel")}
        destructive
        onConfirm={() => owner.confirmReplacement(sessionId)}
      />
    </>
  );
}
